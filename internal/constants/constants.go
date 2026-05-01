// Package constants consolidates all magic strings and shared values used across the codebase.
// This provides a single source of truth for tool names, task types, checklist keys,
// message roles, provider names, default values, and error messages.
//
// Using this package ensures consistency and makes it easier to refactor or update
// shared values throughout the application.
package constants

import "time"

// =============================================================================
// TOOL NAMES
// =============================================================================

// File tool names
const (
	ToolFileRead  = "file_read"
	ToolFileWrite = "file_write"
	ToolFileEdit  = "file_edit"
)

// Web tool names
const (
	ToolWebFetch  = "web_fetch"
)

// Browser tool names
const (
	ToolBrowserNavigate   = "browser_navigate"
	ToolBrowserClick      = "browser_click"
	ToolBrowserType       = "browser_type"
	ToolBrowserScreenshot = "browser_screenshot"
)

// Memory tool names
const (
	ToolMemorySearch = "memory_search"
	ToolMemoryGet    = "memory_get"
	ToolMemoryWrite  = "memory_write"
)

// Lua/Skill tool names
const (
	ToolLuaExec      = "lua_exec"
	ToolSkillList   = "skill_list"
	ToolSkillCreate = "skill_create"
)

// Utility tool names
const (
	ToolMessage              = "message"
	ToolScientificMethodPlan = "scientific_method_plan"
)

// AllTools is a map of all valid tool names for validation purposes
var AllTools = map[string]bool{
	ToolFileRead:             true,
	ToolFileWrite:            true,
	ToolFileEdit:             true,
	ToolWebFetch:             true,
	ToolBrowserNavigate:      true,
	ToolBrowserClick:         true,
	ToolBrowserType:          true,
	ToolBrowserScreenshot:    true,
	ToolMemorySearch:         true,
	ToolMemoryGet:            true,
	ToolMemoryWrite:          true,
	ToolLuaExec:              true,
	ToolSkillList:            true,
	ToolSkillCreate:          true,
	ToolMessage:              true,
	ToolScientificMethodPlan: true,
}

// =============================================================================
// CHECKLIST KEYS
// =============================================================================

// Checklist keys used to track task completion progress
const (
	ChecklistInfoGathered     = "info_gathered"
	ChecklistFilesRead        = "files_read"
	ChecklistComputed         = "computed"
	ChecklistWritten          = "written"
	ChecklistResponseGen      = "response_generated"
	ChecklistNoErrors         = "no_errors"
	ChecklistTaskCompleted    = "task_completed"
	ChecklistExternalData     = "external_data_retrieved"
	ChecklistContentRetrieved = "content_retrieved"
	ChecklistResultGenerated  = "result_generated"
	ChecklistFilesSaved       = "files_saved"
)

// =============================================================================
// TASK TYPES
// =============================================================================

// Intent task types - used to categorize user requests
const (
	TaskInformationGathering = "information_gathering"
	TaskFileAccess           = "file_access"
	TaskComputation          = "computation"
	TaskFileCreation         = "file_creation"
	TaskConversation         = "conversation"
)

// =============================================================================
// MESSAGE ROLES
// =============================================================================

// Message roles for LLM communication
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

// =============================================================================
// PROVIDER NAMES
// =============================================================================

// LLM provider identifiers
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderOllama    = "ollama"
)

// =============================================================================
// DEFAULT MODEL NAMES
// =============================================================================

// Default model identifiers for each provider
const (
	DefaultModelGPT4   = "gpt-4o"
	DefaultModelClaude = "claude-3-5-sonnet-20241022"
)

// =============================================================================
// CONFIG DEFAULTS
// =============================================================================

// Default configuration values
const (
	DefaultTemperature       = 0.7
	DefaultTimeout           = 120 * time.Second
	DefaultErrorThreshold    = 3
	DefaultBootstrapMaxChars = 20000
	DefaultLuaTimeout        = 30 * time.Second
	DefaultLuaMemoryLimit    = 64 // MB
)

// =============================================================================
// ERROR MESSAGES
// =============================================================================

// Error messages for tool validation
const (
	ErrorPathRequired              = "path required"
	ErrorNameRequired              = "name required"
	ErrorQueryRequired             = "query required"
	ErrorURLRequired               = "url required"
	ErrorSelectorRequired          = "selector required"
	ErrorScriptRequired            = "script required"
	ErrorContentRequired           = "content required"
	ErrorUserRequestRequired       = "user_request required"
	ErrorPathAndContentRequired    = "path and content required"
	ErrorPathSearchReplaceRequired = "path, search, replace required"
	ErrorSelectorAndTextRequired   = "selector and text required"
)

// =============================================================================
// INJECT MODES
// =============================================================================

// Agent bootstrap injection modes
const (
	InjectModeEveryTurn = "every-turn"
	InjectModeOnce      = "once"
)

// =============================================================================
// SCIENTIFIC METHOD STEPS
// =============================================================================

// Scientific method workflow steps
const (
	StepObserve     = "observe"
	StepHypothesize = "hypothesize"
	StepExperiment  = "experiment"
	StepAnalyze     = "analyze"
	StepConclude    = "conclude"
)

// =============================================================================
// SCHEMA FIELD NAMES
// =============================================================================

// JSON field names for scientific_method_plan output
const (
	SchemaFieldStep            = "step"
	SchemaFieldIntent          = "intent"
	SchemaFieldToolsNeeded     = "tools_needed"
	SchemaFieldPredictions     = "predictions"
	SchemaFieldSuccessCriteria = "success_criteria"
	SchemaFieldReady           = "ready"
	SchemaFieldReasoning       = "reasoning"
)

// =============================================================================
// ENVIRONMENT VARIABLE NAMES
// =============================================================================

// Environment variable names for configuration
const (
	EnvOpenAIAPIKey    = "OPENAI_API_KEY"
	EnvOpenAIBaseURL   = "OPENAI_BASE_URL"
	EnvOpenAIModel     = "OPENAI_MODEL"
	EnvLLMProvider     = "LLM_PROVIDER"
	EnvAnthropicAPIKey = "ANTHROPIC_API_KEY"
)

// =============================================================================
// DEFAULT PATHS
// =============================================================================

// Default workspace and configuration paths
const (
	DefaultWorkspacePath = "~/.naurvis/workspace"
	DefaultConfigPath    = "~/.naurvis/config.yaml"
)

// =============================================================================
// TIME-RELATED KEYWORDS
// =============================================================================

// Keywords that indicate time-related queries (skip schema phase)
var TimeQueryKeywords = []string{"time", "date"}

// =============================================================================
// INTENT PATTERNS
// =============================================================================

// Patterns for detecting user intent
var (
	// Patterns for information gathering intent
	InfoPatterns = []string{"search", "find", "look up", "what is"}

	// Patterns for file access intent
	FilePatterns = []string{"read", "file", "show", "list"}

	// Patterns for computation intent
	ComputePatterns = []string{"calculate", "compute", "run", "execute", "script"}

	// Patterns for file creation intent
	CreatePatterns = []string{"create", "write", "make", "add"}

	// Patterns for conversational intent
	ChatPatterns = []string{"hello", "hi", "how are", "thanks", "thank you"}
)

// =============================================================================
// SUCCESS CRITERIA MAPPING
// =============================================================================

// Map intent flags to their corresponding checklist keys
var IntentToChecklistMap = map[string][]string{
	"NeedsInfo":    {ChecklistInfoGathered, ChecklistExternalData},
	"NeedsFile":    {ChecklistFilesRead, ChecklistContentRetrieved},
	"NeedsCompute": {ChecklistComputed, ChecklistResultGenerated},
	"NeedsCreate":  {ChecklistWritten, ChecklistFilesSaved},
	"IsChat":       {ChecklistResponseGen},
}

// =============================================================================
// TOOL TO INTENT MAPPING
// =============================================================================

// Map task types to required tools
var TaskToToolsMap = map[string][]string{
	TaskInformationGathering: {"memory_search"},
	TaskFileAccess:           {"file_read", "dir_list"},
	TaskComputation:         {"lua_exec"},
	TaskFileCreation:         {"file_write", "git_commit"},
	TaskConversation:        {},
}

// =============================================================================
// PREDICTIONS BY TASK TYPE
// =============================================================================

// Predicted outcomes for each task type
var TaskPredictions = map[string][]string{
	TaskInformationGathering: {"External information will be retrieved", "Relevant data will be found"},
	TaskFileAccess:           {"Files will be read successfully", "Content will be displayed"},
	TaskComputation:          {"Computation will execute", "Result will be returned"},
	TaskFileCreation:         {"Files will be created/modified", "Changes will be saved"},
	TaskConversation:         {"Response will be generated"},
}

// =============================================================================
// PROVIDER BASE URLs
// =============================================================================

// Default base URLs for LLM providers
var ProviderBaseURLs = map[string]string{
	ProviderOpenAI:    "", // Uses default OpenAI endpoint
	ProviderAnthropic: "https://api.anthropic.com/v1",
	ProviderOllama:    "http://localhost:11434/v1",
}
