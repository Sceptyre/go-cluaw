package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/sceptye/go-cluaw/internal/llm"
	"github.com/sceptye/go-cluaw/internal/logger"
	"github.com/sceptye/go-cluaw/internal/lua"
	"github.com/sceptye/go-cluaw/internal/memory"
	"github.com/sceptye/go-cluaw/internal/scheduler"
	"github.com/sceptye/go-cluaw/internal/self_improve"
)

// =============================================================================
// Types
// =============================================================================

// Registry manages all available tools and their execution.
// It acts as a central hub for tool dispatching, error tracking, and skill management.
type Registry struct {
	workspace      string
	file           *FileTool
	web            *WebTool
	browser        *BrowserTool
	memory         *memory.Get
	self           *self_improve.Creator
	detector       *self_improve.Detector
	luaBox         *lua.Sandbox
	skills         *lua.SkillLoader
	llmCli         *llm.Client
	log            *logger.Logger
	scheduler      *scheduler.Scheduler
	discordAdapter *discordgo.Session
	discordChannel string
}

// ToolResult represents the outcome of a tool execution.
type ToolResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// toolDefinition holds metadata for each tool used in documentation generation.
type toolDefinition struct {
	name        string
	description string
	args        string
}

// =============================================================================
// Tool Definitions - Used for dynamic documentation generation
// =============================================================================

// toolDocsMap defines all available tools and their documentation.
// This allows GetToolDocs() to generate documentation dynamically.
var toolDocsMap = map[string]toolDefinition{
	// File tools
	"file_read": {
		name:        "file_read",
		description: "Read a file from the workspace.",
		args:        `{"path": "file.txt"}`,
	},
	"file_write": {
		name:        "file_write",
		description: "Write content to a file in the workspace.",
		args:        `{"path": "file.txt", "content": "..."}`,
	},
	"file_edit": {
		name:        "file_edit",
		description: "Edit a file with search/replace.",
		args:        `{"path": "file.txt", "search": "old", "replace": "new"}`,
	},
	// Web tools
	"web_fetch": {
		name:        "web_fetch",
		description: "Fetch a web page.",
		args:        `{"url": "https://..."}`,
	},
	// Browser tools
	"browser_navigate": {
		name:        "browser_navigate",
		description: "Navigate browser to URL.",
		args:        `{"url": "https://..."}`,
	},
	"browser_click": {
		name:        "browser_click",
		description: "Click element in browser.",
		args:        `{"selector": "#button"}`,
	},
	"browser_type": {
		name:        "browser_type",
		description: "Type into element in browser.",
		args:        `{"selector": "#input", "text": "..."}`,
	},
	"browser_screenshot": {
		name:        "browser_screenshot",
		description: "Capture a screenshot of the current browser state.",
		args:        "No args",
	},
	// Memory tools
	"memory_search": {
		name:        "memory_search",
		description: "Search memory using grep.",
		args:        `{"query": "term"}`,
	},
	"memory_get": {
		name:        "memory_get",
		description: "Get memory by reference, tag, or date.",
		args:        `{"reference": "file.md"} or {"tag": "tag"} or {"date": "2026-04-01"}`,
	},
	"memory_write": {
		name:        "memory_write",
		description: "Write to today's memory.",
		args:        `{"entry": "note text"}`,
	},
	// Lua tools
	"lua_exec": {
		name:        "lua_exec",
		description: "Execute Lua code in sandbox.",
		args:        `{"script": "print(1+1)"}`,
	},
	// Skill tools
	"skill_list": {
		name:        "skill_list",
		description: "List available skills.",
		args:        "No args",
	},
	"skill_create": {
		name:        "skill_create",
		description: "Create a new skill.",
		args:        `{"name": "skill-name", "description": "...", "instructions": "..."}`,
	},
	// Planning tool
	"scientific_method_plan": {
		name:        "scientific_method_plan",
		description: "Generate structured plan schema for the scientific method workflow. This tool enforces schema output for decision making.",
		args:        `{"user_request": "..."}`,
	},
	// Schedule tools
	"schedule_list": {
		name:        "schedule_list",
		description: "List all scheduled tasks.",
		args:        "No args",
	},
	// Utility tools
	"message": {
		name:        "message",
		description: "Return a message content (passthrough).",
		args:        `{"content": "..."}`,
	},
}

// =============================================================================
// Constructor
// =============================================================================

// NewRegistry creates a new Registry with all tool dependencies initialized.
// Parameters:
//   - workspace: The workspace directory path
//   - luaBox: Lua sandbox for script execution
//   - skills: Skill loader for managing available skills
//   - llmCli: LLM client for advanced planning
//   - log: Logger for debugging output
//   - sched: Scheduler for managing scheduled tasks
func NewRegistry(workspace string, luaBox *lua.Sandbox, skills *lua.SkillLoader, llmCli *llm.Client, log *logger.Logger, sched *scheduler.Scheduler) *Registry {
	return &Registry{
		workspace: workspace,
		file:      NewFileTool(workspace),
		web:       NewWebTool(),
		browser:   NewBrowserTool(),
		memory:    memory.NewGet(workspace),
		self:      self_improve.NewCreator(workspace),
		detector:  self_improve.NewDetector(workspace, 3),
		luaBox:    luaBox,
		skills:    skills,
		llmCli:    llmCli,
		log:       log,
		scheduler: sched,
	}
}

// =============================================================================
// Public Methods
// =============================================================================

// Execute dispatches a tool call to the appropriate handler based on the tool name.
// It parses the JSON arguments and routes to category-specific execution functions.
//
// Parameters:
//   - tool: The name of the tool to execute
//   - argsjson: JSON-encoded arguments for the tool
//   - sessionID: The current session identifier for error tracking
//
// Returns:
//   - string: The tool's output or result
//   - error: Any error that occurred during execution
func (registry *Registry) Execute(tool string, argsjson string, sessionID string) (string, error) {
	args := registry.parseArgs(argsjson)

	switch tool {
	// File tools
	case "file_read":
		return registry.executeFileRead(args)
	case "file_write":
		return registry.executeFileWrite(args)
	case "file_edit":
		return registry.executeFileEdit(args)

	// Web tools
	case "web_fetch":
		return registry.executeWebFetch(args)

	// Browser tools
	case "browser_navigate":
		return registry.executeBrowserNavigate(args)
	case "browser_click":
		return registry.executeBrowserClick(args)
	case "browser_type":
		return registry.executeBrowserType(args)
	case "browser_screenshot":
		return registry.executeBrowserScreenshot(args)

	// Memory tools
	case "memory_search":
		return registry.executeMemorySearch(args)
	case "memory_get":
		return registry.executeMemoryGet(args)
	case "memory_write":
		return registry.executeMemoryWrite(args)

	// Lua tools
	case "lua_exec":
		return registry.executeLuaExec(args)

	// Skill tools
	case "skill_list":
		return registry.executeSkillList(args)
	case "skill_create":
		return registry.executeSkillCreate(args)

	// Planning tools
	case "scientific_method_plan":
		return registry.executeScientificMethodPlan(args)

	// Utility tools
	case "message":
		return registry.executeMessage(args)

	// Schedule tools
	case "schedule_list":
		return registry.executeScheduleList(args)

	default:
		return "", fmt.Errorf("unknown tool: %s", tool)
	}
}

// RecordError logs an error that occurred during tool execution.
// Used for tracking failure patterns and generating improvement suggestions.
func (registry *Registry) RecordError(tool, errorMsg, sessionID string) error {
	return registry.detector.RecordError(tool, errorMsg, sessionID)
}

// GetGap retrieves any known gap or weakness for a specific tool.
// Returns the gap description and whether one exists.
func (registry *Registry) GetGap(tool string) (string, bool) {
	return registry.detector.GetGap(tool)
}

// CreateSkillFromGap creates a new skill to address a specific gap in tool capabilities.
func (registry *Registry) CreateSkillFromGap(gap, description string) error {
	return registry.self.CreateFromGap(gap, description)
}

// EvaluateSkill runs evaluation metrics on a skill to measure its effectiveness.
func (registry *Registry) EvaluateSkill(skillName string) (self_improve.SkillScore, error) {
	eval := self_improve.NewEvaluator(registry.workspace)
	return eval.Evaluate(skillName)
}

// ShouldRollback determines whether a skill should be rolled back based on poor performance.
func (registry *Registry) ShouldRollback(skillName string) bool {
	eval := self_improve.NewEvaluator(registry.workspace)
	return eval.ShouldRollback(skillName)
}

// SetScheduler sets the scheduler for the registry.
func (registry *Registry) SetScheduler(sched *scheduler.Scheduler) {
	registry.scheduler = sched
}

// SetDiscord sets the Discord session adapter for the registry.
func (registry *Registry) SetDiscord(session *discordgo.Session) {
	registry.discordAdapter = session
}

// SetChannel sets the Discord channel ID for sending messages.
func (registry *Registry) SetChannel(channelID string) {
	registry.discordChannel = channelID
}

// Close cleans up resources held by the registry, particularly browser connections.
func (registry *Registry) Close() error {
	return registry.browser.Close()
}

// GetToolDocs generates markdown documentation for all available tools.
// It dynamically builds the documentation from the toolDocsMap.
func (registry *Registry) GetToolDocs() string {
	var sb strings.Builder

	sb.WriteString("## Tool Calling Instructions\n")
	sb.WriteString("- When you need to use a tool, the tool definitions are sent to you automatically.\n")
	sb.WriteString("- Simply return the function name and arguments - the API handles the format via the tool_calls parameter.\n")
	sb.WriteString("- Do NOT output JSON as text - use structured function calls.\n\n")
	sb.WriteString("## Tools\n\n")

	// Generate documentation from the tool map
	for _, def := range toolDocsMap {
		sb.WriteString(fmt.Sprintf("### %s\n", def.name))
		sb.WriteString(fmt.Sprintf("%s\n", def.description))
		sb.WriteString(fmt.Sprintf("Args: %s\n\n", def.args))
	}

	// Add special documentation for scientific_method_plan return format
	sb.WriteString("### scientific_method_plan\n")
	sb.WriteString("Returns: {\"step\": \"experiment\", \"intent\": \"...\", \"tools_needed\": [...], \"predictions\": [...], \"success_criteria\": [...], \"ready\": bool, \"reasoning\": \"...\"}\n")

	return sb.String()
}

// =============================================================================
// Private Methods - Argument Parsing
// =============================================================================

// parseArgs converts JSON string arguments into a map for easy access.
func (registry *Registry) parseArgs(argsjson string) map[string]interface{} {
	if argsjson == "" {
		return nil
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsjson), &args); err != nil {
		return nil
	}
	return args
}

// =============================================================================
// Private Methods - File Tool Handlers
// =============================================================================

// executeFileRead handles the file_read tool.
// Reads content from a file at the specified path.
func (registry *Registry) executeFileRead(args map[string]interface{}) (string, error) {
	filePath, _ := args["path"].(string)
	if filePath == "" {
		return "", fmt.Errorf("path required")
	}
	return registry.file.Read(filePath)
}

// executeFileWrite handles the file_write tool.
// Writes content to a file at the specified path.
func (registry *Registry) executeFileWrite(args map[string]interface{}) (string, error) {
	filePath, _ := args["path"].(string)
	fileContent, _ := args["content"].(string)
	if filePath == "" || fileContent == "" {
		return "", fmt.Errorf("path and content required")
	}
	return registry.file.Write(filePath, fileContent)
}

// executeFileEdit handles the file_edit tool.
// Edits a file by replacing search string with replace string.
func (registry *Registry) executeFileEdit(args map[string]interface{}) (string, error) {
	filePath, _ := args["path"].(string)
	search, _ := args["search"].(string)
	replace, _ := args["replace"].(string)
	if filePath == "" || search == "" {
		return "", fmt.Errorf("path, search, replace required")
	}
	return registry.file.Edit(filePath, search, replace)
}

// =============================================================================
// Private Methods - Web Tool Handlers
// =============================================================================

// executeWebFetch handles the web_fetch tool.
// Fetches and returns content from a URL.
func (registry *Registry) executeWebFetch(args map[string]interface{}) (string, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url required")
	}
	return registry.web.Fetch(url)
}

// =============================================================================
// Private Methods - Browser Tool Handlers
// =============================================================================

// executeBrowserNavigate handles the browser_navigate tool.
// Navigates the browser to the specified URL.
func (registry *Registry) executeBrowserNavigate(args map[string]interface{}) (string, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url required")
	}
	return registry.browser.Navigate(url)
}

// executeBrowserClick handles the browser_click tool.
// Clicks an element in the browser matching the CSS selector.
func (registry *Registry) executeBrowserClick(args map[string]interface{}) (string, error) {
	selector, _ := args["selector"].(string)
	if selector == "" {
		return "", fmt.Errorf("selector required")
	}
	return registry.browser.Click(selector)
}

// executeBrowserType handles the browser_type tool.
// Types text into an element matching the CSS selector.
func (registry *Registry) executeBrowserType(args map[string]interface{}) (string, error) {
	selector, _ := args["selector"].(string)
	text, _ := args["text"].(string)
	if selector == "" || text == "" {
		return "", fmt.Errorf("selector and text required")
	}
	return registry.browser.Type(selector, text)
}

// executeBrowserScreenshot handles the browser_screenshot tool.
// Captures a screenshot of the current browser state.
func (registry *Registry) executeBrowserScreenshot(args map[string]interface{}) (string, error) {
	data, err := registry.browser.Screenshot()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("[screenshot: %d bytes]", len(data)), nil
}

// =============================================================================
// Private Methods - Memory Tool Handlers
// =============================================================================

// executeMemorySearch handles the memory_search tool.
// Searches memory/index for matching entries.
func (registry *Registry) executeMemorySearch(args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query required")
	}
	m := memory.NewIndex(registry.workspace)
	return m.Search(query)
}

// executeMemoryGet handles the memory_get tool.
// Retrieves memory entries by reference, tag, or date.
func (registry *Registry) executeMemoryGet(args map[string]interface{}) (string, error) {
	ref, _ := args["reference"].(string)
	tag, _ := args["tag"].(string)
	date, _ := args["date"].(string)

	if ref != "" {
		return registry.memory.ByReference(ref)
	}
	if tag != "" {
		return registry.memory.ByTag(tag)
	}
	if date != "" {
		return registry.memory.ByDate(date)
	}
	return registry.memory.Today()
}

// executeMemoryWrite handles the memory_write tool.
// Appends an entry to today's memory file.
func (registry *Registry) executeMemoryWrite(args map[string]interface{}) (string, error) {
	entry, _ := args["entry"].(string)
	store := memory.NewStore(registry.workspace)
	if err := store.EnsureTodayExists(); err != nil {
		return "", err
	}
	if err := store.AppendToToday(entry); err != nil {
		return "", err
	}
	return "OK: memory updated", nil
}

// =============================================================================
// Private Methods - Lua Tool Handlers
// =============================================================================

// executeLuaExec handles the lua_exec tool.
// Executes Lua script in the sandboxed environment.
func (registry *Registry) executeLuaExec(args map[string]interface{}) (string, error) {
	script, _ := args["script"].(string)
	if script == "" {
		return "", fmt.Errorf("script required")
	}
	return registry.luaBox.Execute(script, "")
}

// =============================================================================
// Private Methods - Skill Tool Handlers
// =============================================================================

// executeSkillList handles the skill_list tool.
// Lists all available skills in the system.
func (registry *Registry) executeSkillList(args map[string]interface{}) (string, error) {
	skills, err := registry.skills.List()
	if err != nil {
		return "", err
	}
	if len(skills) == 0 {
		return "No skills available", nil
	}
	result := "Available skills:\n"
	for _, s := range skills {
		result += fmt.Sprintf("- %s\n", s)
	}
	return result, nil
}

// executeSkillCreate handles the skill_create tool.
// Creates a new skill from the provided specification.
func (registry *Registry) executeSkillCreate(args map[string]interface{}) (string, error) {
	name, _ := args["name"].(string)
	description, _ := args["description"].(string)
	instructions, _ := args["instructions"].(string)

	if name == "" {
		return "", fmt.Errorf("name required")
	}
	if description == "" {
		description = "Auto-created skill"
	}
	if instructions == "" {
		instructions = "Execute the requested task"
	}

	skill := self_improve.SkillSpec{
		Name:         name,
		Description:  description,
		Instructions: instructions,
		Runtime:      "lua",
	}
	return "OK: skill created: " + name, registry.self.Create(skill)
}

// =============================================================================
// Private Methods - Planning Tool Handlers
// =============================================================================

// executeScientificMethodPlan handles the scientific_method_plan tool.
// Generates a structured plan using the scientific method workflow.
func (registry *Registry) executeScientificMethodPlan(args map[string]interface{}) (string, error) {
	userRequest, _ := args["user_request"].(string)
	if userRequest == "" {
		return "", fmt.Errorf("user_request required")
	}

	// Call LLM to analyze the user request with the scientific method prompt
	planOutput, err := registry.analyzeWithLLM(userRequest)
	if err != nil {
		// Fallback to basic structured output if LLM fails
		planOutput = fallbackPlan(userRequest)
	}

	// Debug log the raw LLM output
	registry.log.Debug("scientific_method_plan LLM raw output: %s", planOutput)

	return planOutput, nil
}

// =============================================================================
// Private Methods - Utility Tool Handlers
// =============================================================================

// executeMessage handles the message tool.
// Simply returns the provided content (passthrough).
// If a Discord channel is configured, also sends the message to Discord.
func (registry *Registry) executeMessage(args map[string]interface{}) (string, error) {
	content, _ := args["content"].(string)

	// Send to Discord if channel is configured
	if registry.discordChannel != "" && registry.discordAdapter != nil {
		if _, err := registry.discordAdapter.ChannelMessageSend(registry.discordChannel, content); err != nil {
			registry.log.Error("Failed to send message to Discord: %v", err)
		} else {
			registry.log.Debug("Message sent to Discord channel %s", registry.discordChannel)
		}
	}

	return content, nil
}

// =============================================================================
// Private Methods - LLM Analysis
// =============================================================================

// analyzeWithLLM uses the LLM to analyze a user request and generate a structured plan.
// Falls back to keyword-based planning if LLM is unavailable or fails.
func (registry *Registry) analyzeWithLLM(userRequest string) (string, error) {
	if registry.llmCli == nil {
		return fallbackPlan(userRequest), nil
	}

	systemPrompt := `You are a scientific method reasoning engine. Analyze this user request and produce a structured plan.

User request: "{user_request}"

Output ONLY valid JSON:
{
  "step": "observe|hypothesize|experiment|analyze|conclude",
  "intent": "what user wants",
  "tools_needed": ["actual tool names that exist"], 
  "predictions": ["expected outcomes"],
  "success_criteria": ["checklist items"],
  "ready": true/false,
  "reasoning": "why this plan"
}

Use ONLY these tools: file_read, file_write, lua_exec, skill_list, memory_search`

	prompt := strings.Replace(systemPrompt, "{user_request}", userRequest, 1)

	resp, _, err := registry.llmCli.Chat(prompt, []llm.Message{})
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	// Try to extract JSON from response
	resp = strings.TrimSpace(resp)
	// Remove markdown code blocks if present
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	// Validate it's valid JSON
	var test map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &test); err != nil {
		// Not valid JSON, wrap it
		planOutput := map[string]interface{}{
			"step":             "experiment",
			"intent":           "information_gathering",
			"tools_needed":     []string{"memory_search"},
			"predictions":      []string{"Information will be retrieved"},
			"success_criteria": []string{"info_gathered", "no_errors"},
			"ready":            true,
			"reasoning":        "Fallback due to LLM response format: " + resp,
		}
		outputJSON, _ := json.Marshal(planOutput)
		return string(outputJSON), nil
	}

	return resp, nil
}

// =============================================================================
// Private Methods - Fallback Planning
// =============================================================================

// fallbackPlan provides a simple keyword-based planning when LLM is unavailable.
// It analyzes the user request and determines intent, tools, predictions, and success criteria.
func fallbackPlan(userRequest string) string {
	// Determine the user's intent based on keywords
	intent := determineIntent(userRequest)

	// Determine which tools are needed based on intent
	toolsNeeded := determineToolsNeeded(intent)

	// Determine expected predictions based on intent
	predictions := determinePredictions(intent)

	// Determine success criteria based on intent
	successCriteria := determineCriteria(intent)

	// Determine if ready based on having tools available
	ready := len(toolsNeeded) > 0
	reasoning := "Ready to proceed with experiment"
	if !ready {
		reasoning = "No tools identified for this request type"
	}

	// Build structured output
	planOutput := map[string]interface{}{
		"step":             "experiment",
		"intent":           intent,
		"tools_needed":     toolsNeeded,
		"predictions":      predictions,
		"success_criteria": successCriteria,
		"ready":            ready,
		"reasoning":        reasoning,
	}

	outputJSON, err := json.Marshal(planOutput)
	if err != nil {
		return `{"step":"experiment","intent":"information_gathering","tools_needed":["memory_search"],"predictions":["Information retrieved"],"success_criteria":["completed"],"ready":true,"reasoning":"fallback"}`
	}

	return string(outputJSON)
}

// determineIntent analyzes the user request to determine the intent.
// Looks for keywords like "create", "read", "calculate", "search" etc.
func determineIntent(userRequest string) string {
	lowerReq := strings.ToLower(userRequest)

	if strings.Contains(lowerReq, "create") || strings.Contains(lowerReq, "write") || strings.Contains(lowerReq, "save") {
		return "file_creation"
	}
	if strings.Contains(lowerReq, "read") || strings.Contains(lowerReq, "file") || strings.Contains(lowerReq, "list") {
		return "file_access"
	}
	if strings.Contains(lowerReq, "calculate") || strings.Contains(lowerReq, "compute") || strings.Contains(lowerReq, "run") {
		return "computation"
	}
	if strings.Contains(lowerReq, "search") || strings.Contains(lowerReq, "find") || strings.Contains(lowerReq, "what") {
		return "information_gathering"
	}

	return "information_gathering"
}

// determineToolsNeeded returns the appropriate tools based on the determined intent.
func determineToolsNeeded(intent string) []string {
	switch intent {
	case "file_creation":
		return []string{"file_write", "git_commit"}
	case "file_access":
		return []string{"file_read", "dir_list"}
	case "computation":
		return []string{"lua_exec"}
	case "information_gathering":
		return []string{"memory_search"}
	default:
		return []string{}
	}
}

// determinePredictions returns expected outcomes based on the determined intent.
func determinePredictions(intent string) []string {
	switch intent {
	case "file_creation":
		return []string{"Files will be created/modified", "Changes will be saved"}
	case "file_access":
		return []string{"Files will be read successfully", "Content will be displayed"}
	case "computation":
		return []string{"Computation will execute", "Result will be returned"}
	case "information_gathering":
		return []string{"External information will be retrieved", "Relevant data will be found"}
	default:
		return []string{"Task will complete successfully"}
	}
}

// determineCriteria returns success criteria based on the determined intent.
func determineCriteria(intent string) []string {
	switch intent {
	case "file_creation":
		return []string{"written", "files_saved", "no_errors"}
	case "file_access":
		return []string{"files_read", "content_retrieved", "no_errors"}
	case "computation":
		return []string{"computed", "result_generated", "no_errors"}
	case "information_gathering":
		return []string{"info_gathered", "external_data_retrieved", "no_errors"}
	default:
		return []string{"task_completed", "no_errors"}
	}
}

// =============================================================================
// Private Methods - Schedule Tool Handlers
// =============================================================================

// executeScheduleList handles the schedule_list tool.
// Lists all scheduled tasks.
func (registry *Registry) executeScheduleList(args map[string]interface{}) (string, error) {
	return registry.listScheduleTasks()
}

// addScheduleTask adds a new scheduled task.
func (registry *Registry) addScheduleTask(name, schedule, message string) (string, error) {
	if registry.scheduler == nil {
		return "", fmt.Errorf("scheduler not available")
	}
	if err := registry.scheduler.AddTask(name, schedule, message); err != nil {
		return "", err
	}
	return fmt.Sprintf("OK: scheduled task '%s' added with schedule '%s'", name, schedule), nil
}

// removeScheduleTask removes a scheduled task.
func (registry *Registry) removeScheduleTask(name string) (string, error) {
	if registry.scheduler == nil {
		return "", fmt.Errorf("scheduler not available")
	}
	if err := registry.scheduler.RemoveTask(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("OK: scheduled task '%s' removed", name), nil
}

// listScheduleTasks lists all scheduled tasks.
func (registry *Registry) listScheduleTasks() (string, error) {
	if registry.scheduler == nil {
		return "No scheduler available", nil
	}
	tasks := registry.scheduler.ListTasks()
	if len(tasks) == 0 {
		return "No scheduled tasks", nil
	}
	result := "Scheduled tasks:\n"
	for _, t := range tasks {
		enabled := "disabled"
		if t.Enabled {
			enabled = "enabled"
		}
		result += fmt.Sprintf("- %s: %s (%s, next: %s)\n", t.Name, t.Schedule, enabled, t.NextRun)
	}
	return result[:len(result)-1], nil // Remove trailing newline
}

// LuaBox returns the Lua sandbox for external access (e.g., wiring scheduler)
func (r *Registry) LuaBox() *lua.Sandbox {
	return r.luaBox
}
