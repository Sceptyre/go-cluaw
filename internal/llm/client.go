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
var knownTools = map[string]bool{
	"file_read":              true,
	"file_write":             true,
	"file_edit":              true,
	"web_fetch":              true,
	"browser_navigate":       true,
	"browser_click":          true,
	"browser_type":           true,
	"browser_screenshot":     true,
	"memory_search":          true,
	"memory_get":             true,
	"memory_write":           true,
	"lua_exec":               true,
	"skill_list":             true,
	"skill_create":           true,
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

// getTools returns all available tools for function calling.
// Tools are grouped by category: File, Web, Browser, Memory, Lua, Skills, and Utility.
// This is used by OpenAI and Ollama providers which support native function calling.
func (c *Client) getTools() []openai.ChatCompletionToolUnionParam {
	tools := []openai.ChatCompletionToolUnionParam{}

	tools = append(tools, c.fileTools()...)
	tools = append(tools, c.webTools()...)
	tools = append(tools, c.browserTools()...)
	tools = append(tools, c.memoryTools()...)
	tools = append(tools, c.luaTools()...)
	tools = append(tools, c.skillTools()...)
	tools = append(tools, c.utilityTools()...)

	return tools
}

// fileTools returns file operation tools: read, write, edit.
func (c *Client) fileTools() []openai.ChatCompletionToolUnionParam {
	descRead := "Read a file from the workspace"
	descWrite := "Write content to a file"
	descEdit := "Edit a file with search and replace"

	fileReadParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]string{
				"type":        "string",
				"description": "File path relative to workspace",
			},
		},
		"required": []string{"path"},
	}

	fileWriteParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]string{
				"type":        "string",
				"description": "File path relative to workspace",
			},
			"content": map[string]string{
				"type":        "string",
				"description": "Content to write to file",
			},
		},
		"required": []string{"path", "content"},
	}

	fileEditParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]string{
				"type":        "string",
				"description": "File path relative to workspace",
			},
			"search": map[string]string{
				"type":        "string",
				"description": "Text to search for in the file",
			},
			"replace": map[string]string{
				"type":        "string",
				"description": "Text to replace the search text with",
			},
		},
		"required": []string{"path", "search", "replace"},
	}

	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "file_read",
			Description: openai.String(descRead),
			Parameters:  fileReadParams,
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "file_write",
			Description: openai.String(descWrite),
			Parameters:  fileWriteParams,
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "file_edit",
			Description: openai.String(descEdit),
			Parameters:  fileEditParams,
		}),
	}
}

// webTools returns web fetch tools.
func (c *Client) webTools() []openai.ChatCompletionToolUnionParam {
	descWebFetch := "Fetch web page content"

	webFetchParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]string{
				"type":        "string",
				"description": "URL to fetch",
			},
		},
		"required": []string{"url"},
	}

	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "web_fetch",
			Description: openai.String(descWebFetch),
			Parameters:  webFetchParams,
		}),
	}
}

// browserTools returns browser automation tools: navigate, click, type, screenshot.
func (c *Client) browserTools() []openai.ChatCompletionToolUnionParam {
	descBrowserNav := "Navigate browser to URL"
	descBrowserClick := "Click element by CSS selector"
	descBrowserType := "Type text into element"
	descBrowserScreenshot := "Take a screenshot"

	browserNavigateParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]string{
				"type":        "string",
				"description": "URL to navigate to",
			},
		},
		"required": []string{"url"},
	}

	browserClickParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"selector": map[string]string{
				"type":        "string",
				"description": "CSS selector for the element to click",
			},
		},
		"required": []string{"selector"},
	}

	browserTypeParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"selector": map[string]string{
				"type":        "string",
				"description": "CSS selector for the input element",
			},
			"text": map[string]string{
				"type":        "string",
				"description": "Text to type into the element",
			},
		},
		"required": []string{"selector", "text"},
	}

	browserScreenshotParams := openai.FunctionParameters{
		"type":       "object",
		"properties": map[string]any{},
	}

	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "browser_navigate",
			Description: openai.String(descBrowserNav),
			Parameters:  browserNavigateParams,
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "browser_click",
			Description: openai.String(descBrowserClick),
			Parameters:  browserClickParams,
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "browser_type",
			Description: openai.String(descBrowserType),
			Parameters:  browserTypeParams,
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "browser_screenshot",
			Description: openai.String(descBrowserScreenshot),
			Parameters:  browserScreenshotParams,
		}),
	}
}

// memoryTools returns memory management tools: search, get, write.
func (c *Client) memoryTools() []openai.ChatCompletionToolUnionParam {
	descMemSearch := "Search memory with grep"
	descMemGet := "Get memory by reference, tag, or date"
	descMemWrite := "Write entry to memory"

	memorySearchParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]string{
				"type":        "string",
				"description": "Search query for memory grep",
			},
		},
		"required": []string{"query"},
	}

	memoryGetParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"reference": map[string]string{
				"type":        "string",
				"description": "Memory reference (file path)",
			},
			"tag": map[string]string{
				"type":        "string",
				"description": "Filter by tag",
			},
			"date": map[string]string{
				"type":        "string",
				"description": "Filter by date (YYYY-MM-DD)",
			},
		},
	}

	memoryWriteParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"entry": map[string]string{
				"type":        "string",
				"description": "Entry text to write to memory",
			},
		},
		"required": []string{"entry"},
	}

	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "memory_search",
			Description: openai.String(descMemSearch),
			Parameters:  memorySearchParams,
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "memory_get",
			Description: openai.String(descMemGet),
			Parameters:  memoryGetParams,
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "memory_write",
			Description: openai.String(descMemWrite),
			Parameters:  memoryWriteParams,
		}),
	}
}

// luaTools returns the Lua execution tool.
func (c *Client) luaTools() []openai.ChatCompletionToolUnionParam {
	descExec := "Execute Lua code in the sandbox"

	luaExecParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"script": map[string]string{
				"type":        "string",
				"description": "Lua script to execute",
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

// skillTools returns skill management tools: list, create.
func (c *Client) skillTools() []openai.ChatCompletionToolUnionParam {
	descList := "List all available skills"
	descCreateSkill := "Create a new skill"

	skillListParams := openai.FunctionParameters{
		"type":       "object",
		"properties": map[string]any{},
	}

	skillCreateParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]string{
				"type":        "string",
				"description": "Name of the skill to create",
			},
			"description": map[string]string{
				"type":        "string",
				"description": "Description of the skill",
			},
			"instructions": map[string]string{
				"type":        "string",
				"description": "Instructions for the skill",
			},
		},
		"required": []string{"name"},
	}

	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "skill_list",
			Description: openai.String(descList),
			Parameters:  skillListParams,
		}),
		openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        "skill_create",
			Description: openai.String(descCreateSkill),
			Parameters:  skillCreateParams,
		}),
	}
}

// utilityTools returns miscellaneous utility tools: message, scientific_method_plan.
func (c *Client) utilityTools() []openai.ChatCompletionToolUnionParam {
	descMessage := "Send a message via Discord"
	descScientificMethod := "Generate scientific method plan"

	messageParams := openai.FunctionParameters{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]string{
				"type":        "string",
				"description": "Message content to send",
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
