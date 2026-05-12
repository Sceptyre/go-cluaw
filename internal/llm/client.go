package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// =============================================================================
// Types
// =============================================================================

// Client wraps the LLM provider client and configuration.
type Client struct {
	cfg    Config
	client openai.Client
}

// Config holds the configuration for the LLM client.
type Config struct {
	Provider string // "openai", "anthropic", or "ollama"
	APIKey   string
	Model    string
	BaseURL  string
}

// Message represents a chat message in the conversation.
type Message struct {
	Role      string     `json:"role"`    // "user", "assistant", "system"
	Content   string     `json:"content"` // The message content
	Name      string     `json:"name,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a function call requested by the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the function name and arguments.
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// jsonToolCall is used for parsing JSON tool calls from text content.
type jsonToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Tool defines a function tool that can be called by the LLM.
type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef defines the function schema for a tool.
type FunctionDef struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Parameters  *json.RawMessage `json:"parameters,omitempty"`
}

// =============================================================================
// Constants
// =============================================================================

var defaultTimeout = 120 * time.Second

// knownTools is a set of valid tool names for validation in the JSON fallback parser.
// World-interaction tools have been consolidated into lua_exec Lua modules.
var knownTools = map[string]bool{
	"lua_exec":               true,
	"message":                true,
	"scientific_method_plan": true,
}

// =============================================================================
// Constructor
// =============================================================================

// New creates a new LLM client with the given configuration.
// It automatically loads the API key from environment variables if not provided.
func New(cfg Config) *Client {
	var opts []option.RequestOption

	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = LoadAPIKey(cfg.Provider)
	}

	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	} else {
		opts = append(opts, option.WithAPIKey(os.Getenv("OPENAI_API_KEY")))
	}

	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	} else {
		switch cfg.Provider {
		case "openai":
			// OpenAI uses default base URL
		case "anthropic":
			opts = append(opts, option.WithBaseURL("https://api.anthropic.com/v1"))
		case "ollama":
			opts = append(opts, option.WithBaseURL("http://localhost:11434/v1"))
		}
	}

	client := openai.NewClient(opts...)

	return &Client{cfg: cfg, client: client}
}

// =============================================================================
// Public Methods
// =============================================================================

// Chat sends a chat request to the LLM and returns the response content and any tool calls.
// It supports multiple providers:
//   - OpenAI and Ollama: Support native function calling via the tools parameter
//   - Anthropic: Uses OpenAI-compatible API but does not support function calling in the same way
//
// Returns: content string, tool calls, and any error.
func (c *Client) Chat(systemPrompt string, messages []Message) (string, []ToolCall, error) {
	ctx := context.Background()

	chatMessages := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(systemPrompt)}

	for _, msg := range messages {
		var messageParam openai.ChatCompletionMessageParamUnion
		switch msg.Role {
		case "user":
			messageParam = openai.UserMessage(msg.Content)
		case "assistant":
			messageParam = openai.AssistantMessage(msg.Content)
		case "system":
			messageParam = openai.SystemMessage(msg.Content)
		default:
			messageParam = openai.UserMessage(msg.Content)
		}
		chatMessages = append(chatMessages, messageParam)
	}

	switch c.cfg.Provider {
	case "openai", "ollama":
		// OpenAI and Ollama support native function calling
		return c.doOpenAIChat(ctx, chatMessages)
	case "anthropic":
		// Anthropic uses OpenAI-compatible API but doesn't support function calling
		// in the same way as OpenAI - it returns text only
		return c.doAnthropicChat(ctx, systemPrompt, messages)
	default:
		return "Provider not supported", nil, nil
	}
}

// ChatWithJSONResponse sends a chat request and expects a JSON response.
// It returns the raw JSON string and any error.
func (c *Client) ChatWithJSONResponse(systemPrompt, userMessage string) (string, error) {
	ctx := context.Background()

	// For JSON response, we use the Anthropic approach (text only, no tools)
	// because we want structured JSON output, not tool calls
	chatMessages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(userMessage),
	}

	model := c.cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	params := openai.ChatCompletionNewParams{
		Model:       openai.ChatModel(model),
		Messages:    chatMessages,
		Temperature: openai.Float(0.3), // Lower temperature for structured output
	}

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("chat completion error: %w", err)
	}

	content := ""
	if len(response.Choices) > 0 && response.Choices[0].Message.Content != "" {
		content = response.Choices[0].Message.Content
	}

	return content, nil
}

// LoadAPIKey loads the API key from environment variables.
// It first checks for provider-specific keys (e.g., ANTHROPIC_API_KEY),
// then falls back to OPENAI_API_KEY.
func LoadAPIKey(provider string) string {
	envKey := strings.ToUpper(provider) + "_API_KEY"
	key := os.Getenv(envKey)
	if key != "" {
		return key
	}
	return os.Getenv("OPENAI_API_KEY")
}

// StreamChat is not implemented - requires a Client instance.
func StreamChat(baseURL, apiKey, model, systemPrompt string, messages []Message, onChunk func(string)) error {
	return fmt.Errorf("StreamChat requires a Client instance")
}

// =============================================================================
// Private Methods - OpenAI/Ollama Provider
// =============================================================================

// doOpenAIChat handles chat requests for OpenAI and Ollama providers.
// Both support native function calling via the tools parameter.
func (c *Client) doOpenAIChat(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (string, []ToolCall, error) {
	model := c.cfg.Model
	if model == "" {
		model = "gpt-4o"
	}

	tools := c.getTools()

	params := openai.ChatCompletionNewParams{
		Messages:    messages,
		Model:       openai.ChatModel(model),
		Temperature: openai.Float(0.7),
	}

	if len(tools) > 0 {
		params.Tools = tools
		log.Printf("=== REQUEST TO LLM: %d tools being sent ===", len(tools))
	}

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", nil, fmt.Errorf("chat completion error: %w", err)
	}

	content := ""
	if len(response.Choices) > 0 && response.Choices[0].Message.Content != "" {
		content = response.Choices[0].Message.Content
	}

	var toolCallResults []ToolCall
	// Guard against empty choices - some API responses may have no choices
	if len(response.Choices) == 0 {
		// No choices returned - this can happen with some models/providers
		// Return what we have (possibly empty content)
		return content, toolCallResults, nil
	}
	for _, toolCall := range response.Choices[0].Message.ToolCalls {
		args := json.RawMessage(toolCall.Function.Arguments)
		toolCallResults = append(toolCallResults, ToolCall{
			ID:   toolCall.ID,
			Type: string(toolCall.Type),
			Function: FunctionCall{
				Name:      toolCall.Function.Name,
				Arguments: args,
			},
		})
	}

	// Force tool-only mode: if we got content but no tool calls, log warning
	// but still return the content as fallback after 1 attempt.
	// The agent loop will retry automatically if needed.
	if content != "" && len(toolCallResults) == 0 {
		// Log warning - this means the model didn't use tools
		log.Printf("[WARN] LLM returned text without tool calls - will retry once")
		// Return content anyway but empty toolCallResults - this signals "retry needed"
		// The agent loop should detect this and add a nudging message to retry
	}

	// Ultimate fallback: if both content and toolCalls are empty (model failed to respond)
	// The agent loop will retry with a nudging message

	// Fallback: Some models may return tool calls as JSON in the content field
	// instead of using the native tool_calls field. This attempts to parse
	// such content as a JSON tool call.
	// Note: This is a heuristic and may not always be correct.
	if len(toolCallResults) == 0 && content != "" {
		trimmed := strings.TrimSpace(content)
		if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
			if toolCall, err := parsePotentialToolCall(trimmed); err == nil {
				toolCallResults = append(toolCallResults, *toolCall)
				content = ""
			}
		}
	}

	return content, toolCallResults, nil
}

// =============================================================================
// Private Methods - Anthropic Provider
// =============================================================================

// doAnthropicChat handles chat requests for the Anthropic provider.
// Anthropic does not support function calling in the same way as OpenAI.
// It returns text content only, and tool calls are not supported.
// The function returns nil for tool calls to indicate this limitation.
func (c *Client) doAnthropicChat(ctx context.Context, systemPrompt string, messages []Message) (string, []ToolCall, error) {
	model := c.cfg.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	anthropicMessages := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(systemPrompt)}

	for _, msg := range messages {
		var messageParam openai.ChatCompletionMessageParamUnion
		switch msg.Role {
		case "user":
			messageParam = openai.UserMessage(msg.Content)
		case "assistant":
			messageParam = openai.AssistantMessage(msg.Content)
		case "system":
			messageParam = openai.SystemMessage(msg.Content)
		default:
			messageParam = openai.UserMessage(msg.Content)
		}
		anthropicMessages = append(anthropicMessages, messageParam)
	}

	params := openai.ChatCompletionNewParams{
		Model:       openai.ChatModel(model),
		Messages:    anthropicMessages,
		Temperature: openai.Float(0.7),
	}

	response, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", nil, fmt.Errorf("anthropic chat error: %w", err)
	}

	content := ""
	if len(response.Choices) > 0 && response.Choices[0].Message.Content != "" {
		content = response.Choices[0].Message.Content
	}

	// Anthropic does not support function calling in the same way as OpenAI.
	// Return nil for tool calls to indicate this limitation.
	return content, nil, nil
}

// =============================================================================
// Private Methods - JSON Tool Call Parsing
// =============================================================================

// parsePotentialToolCall attempts to parse content as a JSON tool call.
// This is used as a fallback for models that don't use the native tool_calls field.
// It validates that the "name" field contains a known tool name before accepting it.
func parsePotentialToolCall(content string) (*ToolCall, error) {
	var toolCall jsonToolCall
	if err := json.Unmarshal([]byte(content), &toolCall); err != nil {
		return nil, err
	}

	// Validate that the tool name is not empty
	if toolCall.Name == "" {
		return nil, fmt.Errorf("tool call name is empty")
	}

	// Validate that the tool name is a known tool
	// This prevents false positives from arbitrary JSON objects
	if !knownTools[toolCall.Name] {
		return nil, fmt.Errorf("unknown tool: %s", toolCall.Name)
	}

	id := fmt.Sprintf("call_%s_%d", strings.ReplaceAll(toolCall.Name, "_", ""), time.Now().UnixNano())
	return &ToolCall{
		ID:   id,
		Type: "function",
		Function: FunctionCall{
			Name:      toolCall.Name,
			Arguments: toolCall.Arguments,
		},
	}, nil
}

// =============================================================================
// Private Methods - Tool Definitions
// =============================================================================

// getTools returns the tools exposed to the LLM.
// Only system/orchestration tools are sent directly.
// World interaction is delegated to lua_exec which exposes file, web, browser, memory, skill modules.
func (c *Client) getTools() []openai.ChatCompletionToolUnionParam {
	return append(c.systemTools(), c.luaExecTool()...)
}

// systemTools returns system/orchestration tools: message, scientific_method_plan.
func (c *Client) systemTools() []openai.ChatCompletionToolUnionParam {
	descMessage := "Send a message to the user (call this when you have the final response)"
	descScientificMethod := "Generate a structured scientific method plan"

	messageParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]string{
				"type":        "string",
				"description": "Message content to send to the user",
			},
		},
		"required": []string{"content"},
	}

	scientificMethodPlanParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"user_request": map[string]string{
				"type":        "string",
				"description": "User request to analyze",
			},
		},
		"required": []string{"user_request"},
	}

	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "message",
			Description: openai.String(descMessage),
			Parameters:  messageParams,
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "scientific_method_plan",
			Description: openai.String(descScientificMethod),
			Parameters:  scientificMethodPlanParams,
		}),
	}
}

// luaExecTool returns the Lua execution tool — the primary gateway to all world interaction.
func (c *Client) luaExecTool() []openai.ChatCompletionToolUnionParam {
	descExec := `Execute Lua code in the sandbox. This is your PRIMARY tool for ANY interaction with the world.
All capabilities are accessed through Lua modules:
- file.read/write/edit/list — File operations
- web.fetch/search — Web page fetching and searching
- browser.navigate/click/type/screenshot — Browser automation
- memory.today/get/write/search — Memory management
- skill.list/exec/create — Skill management
- scheduler.add/remove/list — Scheduled tasks
- os.date/time — Date/time utilities
- print() — Capture intermediate output`

	luaExecParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"script": map[string]string{
				"type":        "string",
				"description": "Lua script to execute. Use Lua modules for all operations.",
			},
		},
		"required": []string{"script"},
	}

	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "lua_exec",
			Description: openai.String(descExec),
			Parameters:  luaExecParams,
		}),
	}
}
