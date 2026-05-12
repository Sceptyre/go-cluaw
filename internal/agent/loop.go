package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sceptye/go-cluaw/internal/config"
	"github.com/sceptye/go-cluaw/internal/gitops"
	"github.com/sceptye/go-cluaw/internal/llm"
	"github.com/sceptye/go-cluaw/internal/logger"
	lua "github.com/sceptye/go-cluaw/internal/lua"
	"github.com/sceptye/go-cluaw/internal/memory"
	"github.com/sceptye/go-cluaw/internal/scheduler"
	"github.com/sceptye/go-cluaw/internal/self_improve"
	"github.com/sceptye/go-cluaw/internal/session"
	"github.com/sceptye/go-cluaw/internal/tools"
	"github.com/sceptye/go-cluaw/internal/workspace"
)

// memoryAdapter wraps memory.Get and memory.Store to satisfy lua.MemoryOperator.
type memoryAdapter struct {
	get    *memory.Get
	store  *memory.Store
	wsPath string
}

func (m *memoryAdapter) Today() (string, error)          { return m.get.Today() }
func (m *memoryAdapter) ByReference(ref string) (string, error) { return m.get.ByReference(ref) }
func (m *memoryAdapter) ByTag(tag string) (string, error)       { return m.get.ByTag(tag) }
func (m *memoryAdapter) ByDate(date string) (string, error)     { return m.get.ByDate(date) }
func (m *memoryAdapter) AppendToToday(entry string) error       { return m.store.AppendToToday(entry) }
func (m *memoryAdapter) Search(query string) (string, error) {
	idx := memory.NewIndex(m.wsPath)
	return idx.Search(query)
}

// =============================================================================
// CONSTANTS
// =============================================================================

const maxToolIterations = 25
const maxNudges = 3          // Maximum times we retry with nudge when no tool calls
const maxHistoryMessages = 3 // Only send last 3 messages to execution LLM

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

// Time-related keywords that skip schema phase
var timeQueryKeywords = []string{"time", "date"}

// Intent task types
const (
	TaskInfoGathering = "information_gathering"
	TaskFileAccess    = "file_access"
	TaskComputation   = "computation"
	TaskFileCreation  = "file_creation"
	TaskConversation  = "conversation"
)

// =============================================================================
// SCIENTIFIC METHOD DATA STRUCTURES
// =============================================================================

// Observation represents extracted entities from user request (STEP 1)
// This is the first phase of the scientific method - observing and gathering data
type Observation struct {
	Message      string            // Original user message
	Who          string            // Who is involved
	What         string            // What action is requested
	When         string            // When (time reference)
	Where        string            // Where (location/path reference)
	Why          string            // Why (purpose/reason)
	How          string            // How (method/manner)
	Intent       Intent            // Detected intent from analyzeIntent
	Entities     map[string]string // Extracted key-value entities
	RawVariables string            // Raw extracted variables for debug
	Timestamp    time.Time         // When observation was made
}

// Intent represents the detected user intent from analyzing their message
// Helps determine what tools and approach to use
type Intent struct {
	NeedsInfo     bool     // needs external info (web, memory)
	NeedsFile     bool     // needs file access
	NeedsCompute  bool     // needs computation
	NeedsCreate   bool     // needs to create/write
	IsChat        bool     // just conversational
	Task          string   // what user wants to do
	RequiredTools []string // tools that should be called
	Confidence    float64  // how confident we are
}

// Hypothesis represents the plan to fulfill the request (STEP 2)
// Second phase: forming a hypothesis about what tools to use
type Hypothesis struct {
	Observation     Observation // Source observation
	HypothesisText  string      // "To fulfill request, I need to..."
	ToolsNeeded     []string    // List of required tools/experiments
	Predictions     []string    // Predicted outcomes
	SuccessCriteria []string    // Checklist for success
	Confidence      float64     // How confident we are in the plan
	ExperimentPlan  string      // Execution plan description
}

// ExperimentResult represents the execution results (STEP 3)
// Third phase: conducting experiments (running tools)
type ExperimentResult struct {
	Hypothesis     Hypothesis             // Source hypothesis
	ToolExecutions []ToolExecution        // Results of each tool execution
	RawResults     []string               // Raw results gathered
	Success        bool                   // Whether execution succeeded
	ErrorMessage   string                 // Error if any
	Timestamp      time.Time              // When experiment was run
	Duration       time.Duration          // How long it took
	DataGathered   map[string]interface{} // Structured data collected
}

// ToolExecution tracks individual tool execution
type ToolExecution struct {
	ToolName  string
	Arguments string
	Result    string
	Error     error
	Success   bool
	Duration  time.Duration
	Timestamp time.Time
}

// Analysis represents the comparison of results to predictions (STEP 4)
// Fourth phase: analyzing results against predictions
type Analysis struct {
	ExperimentResult  ExperimentResult // Source experiment results
	Hypothesis        Hypothesis       // Source hypothesis
	Completed         bool             // Was the task completed?
	Gaps              []string         // Missing items/failures
	Learnings         []string         // What we learned
	PredictionsMet    []string         // Which predictions were met
	PredictionsFailed []string         // Which predictions failed
	ChecklistStatus   map[string]bool  // Status of each success criterion
	TotalCriteria     int              // Total success criteria
	MetCriteria       int              // Met criteria count
}

// Conclusion represents the final response (STEP 5)
// Fifth phase: forming conclusions from the experiment
type Conclusion struct {
	Response         string // Final response to user
	NeedsImprovement bool   // Whether self-improvement is needed
	Success          bool   // Whether task was successful
	Suggestion       string // Self-improvement suggestion if failed
	Summary          string // Summary of what was done
}

// FeedbackData contains data for feedback loop (STEP 6)
// Sixth phase: learning from the experiment to improve future performance
type FeedbackData struct {
	Analysis     Analysis
	Hypothesis   Hypothesis
	Conclusion   Conclusion
	ExperimentID string
	Success      bool
	Timestamp    time.Time
}

// =============================================================================
// VALIDATION FUNCTIONS
// =============================================================================

// ValidateIntent checks if the intent has all required fields
func ValidateIntent(intent Intent) error {
	if intent.Task == "" {
		return fmt.Errorf("intent task cannot be empty")
	}
	if intent.Confidence < 0 || intent.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	return nil
}

// ValidateObservation checks if the observation has required data
func ValidateObservation(obs Observation) error {
	if obs.Message == "" {
		return fmt.Errorf("observation message cannot be empty")
	}
	return nil
}

// =============================================================================
// AGENT STRUCT DEFINITION
// =============================================================================

// Agent orchestrates the scientific method workflow to process user requests
type Agent struct {
	cfg       *config.Config
	ws        *workspace.Workspace
	store     *session.Store
	log       *logger.Logger
	llmCli    *llm.Client
	luaBox    *lua.Sandbox
	skills    *lua.SkillLoader
	gitMgr    *gitops.Manager
	tools     *tools.Registry
	memory    *memory.Store
	detector  *self_improve.Detector
	scheduler *scheduler.Scheduler
}

// =============================================================================
// CONSTRUCTOR
// =============================================================================

// New creates a new Agent instance with all required dependencies
func New(cfg *config.Config, ws *workspace.Workspace, store *session.Store, log *logger.Logger) *Agent {
	// Load API key from config or environment
	apiKey := cfg.LLM.APIKey
	if apiKey == "" {
		apiKey = llm.LoadAPIKey(cfg.LLM.Provider)
	}

	workspacePath := cfg.ExpandWorkspace()

	// Create tool instances first (shared between Lua sandbox and Registry)
	fileTool := tools.NewFileTool(workspacePath)
	webTool := tools.NewWebTool()
	browserTool := tools.NewBrowserTool()
	memStore := memory.NewStore(workspacePath)
	memStore.EnsureTodayExists()
	memGet := memory.NewGet(workspacePath)
	memAdapter := &memoryAdapter{
		get:    memGet,
		store:  memStore,
		wsPath: workspacePath,
	}

	// Initialize skill loader
	skills := lua.NewSkillLoader(workspacePath)

	// Initialize Lua sandbox with all tool dependencies
	timeout := time.Duration(cfg.Lua.TimeoutSeconds) * time.Second
	luaBox := lua.NewWithTools(
		timeout,
		[]string{workspacePath},
		nil, // scheduler set later
		fileTool, webTool, browserTool,
		memAdapter, skills,
	)

	// Initialize GitOps manager
	gitMgr := gitops.New(
		workspacePath,
		cfg.GitOps.AutoCommit,
		cfg.GitOps.ErrorThreshold,
		cfg.GitOps.CommitMessagePrefix,
	)

	// Create LLM client for tool registry
	toolLLMCli := llm.New(llm.Config{
		Provider: cfg.LLM.Provider,
		APIKey:   apiKey,
		Model:    cfg.LLM.Model,
		BaseURL:  cfg.LLM.BaseURL,
	})

	// Create tool registry (shares the same tool instances with Lua sandbox)
	toolRegistry := tools.NewRegistry(workspacePath, luaBox, skills, toolLLMCli, log, nil, fileTool, webTool, browserTool)

	// Initialize self-improvement detector
	detector := self_improve.NewDetector(workspacePath, cfg.GitOps.ErrorThreshold)

	return &Agent{
		cfg:   cfg,
		ws:    ws,
		store: store,
		log:   log,
		llmCli: llm.New(llm.Config{
			Provider: cfg.LLM.Provider,
			APIKey:   apiKey,
			Model:    cfg.LLM.Model,
			BaseURL:  cfg.LLM.BaseURL,
		}),
		luaBox:   luaBox,
		skills:   skills,
		gitMgr:   gitMgr,
		tools:    toolRegistry,
		memory:   memStore,
		detector: detector,
	}
}

// =============================================================================
// CONTEXT INFERENCE
// =============================================================================
// inferContext analyzes the user message to determine what context to retrieve
// Returns a ContextRequest with inferred query types, keywords, and confidence
func (a *Agent) inferContext(message string) (*ContextRequest, error) {
	userPrompt := fmt.Sprintf(InferenceUserPromptTemplate, message)

	resp, err := a.llmCli.ChatWithJSONResponse(InferenceSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("inference LLM error: %w", err)
	}

	// Parse JSON response into ContextRequest
	var req ContextRequest
	if err := json.Unmarshal([]byte(resp), &req); err != nil {
		// If JSON parse fails, return a default low-confidence request
		a.log.Warn("Failed to parse inference response: %v", err)
		return &ContextRequest{
			Confidence: 0.0,
		}, nil
	}

	a.log.Debug("Context inference: confidence=%.2f, types=%v, keywords=%v",
		req.Confidence, req.QueryTypes, req.Keywords)

	return &req, nil
}

// retrieveContext retrieves relevant context based on the ContextRequest
// Queries the session store for past messages matching the request
func (a *Agent) retrieveContext(req *ContextRequest, userID string) (*ContextResponse, error) {
	items := []ContextItem{}

	// If no query types specified, return empty response
	if len(req.QueryTypes) == 0 {
		return &ContextResponse{
			Items:      []ContextItem{},
			Summary:    "",
			HasContext: false,
		}, nil
	}

	// Query based on request types
	for _, qt := range req.QueryTypes {
		switch qt {
		case "conversation":
			// Get recent messages from session
			sess, err := a.store.LoadByUser(userID)
			if err == nil && sess != nil {
				// Get last 10 messages
				msgCount := len(sess.Messages)
				start := 0
				if msgCount > 10 {
					start = msgCount - 10
				}
				// Calculate relevance based on recency: newer messages get higher relevance
				// Most recent message gets 1.0, decreasing to 0.5 for oldest in the window
				for i := start; i < msgCount; i++ {
					m := sess.Messages[i]
					positionFromEnd := msgCount - 1 - i
					relevance := 0.5 + (0.5 * float64(positionFromEnd) / float64(msgCount-start-1))
					if relevance > 1.0 {
						relevance = 1.0
					}
					items = append(items, ContextItem{
						Type:      "message",
						Content:   m.Content,
						Source:    m.Role,
						Timestamp: sess.CreatedAt.Format(time.RFC3339),
						Relevance: relevance,
					})
				}
			}
		case "errors":
			// Query memory for errors if available
			if a.memory != nil {
				entries, _ := a.memory.Search("error")
				for _, entry := range entries {
					items = append(items, ContextItem{
						Type:      "error",
						Content:   entry,
						Source:    "memory",
						Relevance: 0.7,
					})
				}
			}
		case "file_operations":
			// Get recent file operations from session
			sess, err := a.store.LoadByUser(userID)
			if err == nil && sess != nil {
				for _, m := range sess.Messages {
					// Look for file-related content
					if strings.Contains(strings.ToLower(m.Content), "file") {
						items = append(items, ContextItem{
							Type:      "file_op",
							Content:   m.Content,
							Source:    m.Role,
							Relevance: 0.6,
						})
					}
				}
			}
		}
	}

	// Also search by keywords if provided
	if len(req.Keywords) > 0 && a.memory != nil {
		for _, kw := range req.Keywords {
			entries, _ := a.memory.Search(kw)
			for _, entry := range entries {
				items = append(items, ContextItem{
					Type:      "keyword",
					Content:   entry,
					Source:    "memory",
					Relevance: 0.8,
				})
			}
		}
	}

	// Format summary from items
	summary := formatContextSummary(items)

	return &ContextResponse{
		Items:      items,
		Summary:    summary,
		HasContext: len(items) > 0,
	}, nil
}

// formatContextSummary formats context items into a readable summary
func formatContextSummary(items []ContextItem) string {
	if len(items) == 0 {
		return ""
	}

	var parts []string
	for _, item := range items {
		// Truncate each item to 200 chars
		content := item.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		parts = append(parts, fmt.Sprintf("[%s] %s", item.Type, content))
	}

	summary := strings.Join(parts, "\n")

	// Truncate total summary if too long
	maxLen := 20000
	if len(summary) > maxLen {
		summary = summary[:maxLen] + "..."
	}

	return summary
}

// =============================================================================
// PUBLIC METHODS
// =============================================================================

// Process handles the main user request through the scientific method workflow
// It coordinates all six phases: Observation, Hypothesis, Experiment, Analysis, Conclusion, Feedback
func (a *Agent) Process(userID, message string) (string, error) {
	a.log.Info("Processing message from user %s", userID)

	// Load or create session
	sess, err := a.store.LoadByUser(userID)
	if err != nil {
		sess = a.store.Create(userID)
	}
	sess.Messages = append(sess.Messages, session.Message{
		Role:    "user",
		Content: message,
	})

	// ============================================
	// CONTEXT INFERENCE STEP (NEW)
	// ============================================
	var inferredContext string
	if a.cfg.Inference.Enabled {
		// Step 1: Get context request from inference LLM
		contextReq, err := a.inferContext(message)
		if err != nil {
			a.log.Warn("Context inference failed: %v, proceeding without context", err)
		} else if contextReq.Confidence >= a.cfg.Inference.ConfidenceThreshold {
			// Step 2: Retrieve context based on request
			contextResp, err := a.retrieveContext(contextReq, userID)
			if err != nil {
				a.log.Warn("Context retrieval failed: %v", err)
			} else if contextResp.HasContext {
				inferredContext = contextResp.Summary
				a.log.Debug("Inferred context: %d items", len(contextResp.Items))
			}
		}
	}
	// ============================================
	// END CONTEXT INFERENCE
	// ============================================

	// ============================================
	// SCIENTIFIC METHOD WORKFLOW
	// ============================================

	// STEP 1: OBSERVATION
	// Observe user request and extract key variables
	observation := a.observe(message)
	a.log.Debug("Observation phase complete: %s", observation.RawVariables)

	// STEP 2: HYPOTHESIS
	// Form hypothesis based on observation
	hypothesis := a.hypothesize(observation)
	a.log.Debug("Hypothesis phase complete: %s", hypothesis.HypothesisText)

	// STEP 3: EXPERIMENT (integrated with tool execution)
	// Determine system prompt with intent awareness
	systemPrompt, err := a.buildSystemPromptWithIntent(observation.Intent, inferredContext)
	if err != nil {
		return "", err
	}

	response, err := a.runToolLoop(systemPrompt, sess)
	if err != nil {
		a.log.Error("Tool loop error: %v", err)
		return "", err
	}

	// Track experiment results
	experiment := ExperimentResult{
		Hypothesis: hypothesis,
		Success:    err == nil,
		Timestamp:  time.Now(),
	}
	experiment.RawResults = append(experiment.RawResults, response)

	// STEP 4: ANALYSIS
	// Compare results to predictions
	analysis := a.analyze(experiment, hypothesis)
	a.log.Debug("Analysis phase complete: completed=%v, gaps=%v", analysis.Completed, analysis.Gaps)

	// STEP 5: CONCLUSION
	// Form response from results
	conclusion := a.conclude(analysis, experiment)
	a.log.Debug("Conclusion phase complete: success=%v, needsImprovement=%v", conclusion.Success, conclusion.NeedsImprovement)

	// STEP 6: FEEDBACK LOOP
	// Record experiment data and trigger improvements
	a.feedback(analysis, hypothesis)

	// ============================================
	// END SCIENTIFIC METHOD WORKFLOW
	// ============================================

	// Save response to session
	sess.Messages = append(sess.Messages, session.Message{
		Role:    "assistant",
		Content: response,
	})

	a.store.Save(sess)

	return response, nil
}

// SelfImprove creates a new skill from self-improvement content
func (a *Agent) SelfImprove(skillName, content string) error {
	skill := &lua.Skill{
		Name:        skillName,
		Description: "Self-created skill",
		Version:     "1.0.0",
		Author:      "cluaw",
		Runtime:     "lua",
		Content:     content,
	}

	if err := a.skills.Save(skill); err != nil {
		return err
	}

	if a.gitMgr != nil {
		return a.gitMgr.CommitSkillChange(skillName, "self-improvement")
	}

	return nil
}

// Close cleans up agent resources
func (a *Agent) Close() error {
	if a.tools != nil {
		return a.tools.Close()
	}
	return nil
}

// Tools returns the tool registry for external access.
func (a *Agent) Tools() *tools.Registry {
	return a.tools
}

// =============================================================================
// SCIENTIFIC METHOD PHASES
// =============================================================================

// STEP 1: OBSERVATION - Extract entities from user request
// This phase analyzes the user's message to understand what they want
func (a *Agent) observe(message string) Observation {
	observation := Observation{
		Message:   message,
		Entities:  make(map[string]string),
		Timestamp: time.Now(),
	}

	// Analyze intent using LLM
	intent := a.analyzeIntent(message)
	observation.Intent = intent

	// Extract variables using pattern matching
	msg := strings.ToLower(message)

	// Extract "Who" - look for user references, names, pronouns
	whoPatterns := []string{
		"i want", "i need", "i would", "my", "user", "please",
		"can you", "could you", "would you", "create a",
	}
	for _, pattern := range whoPatterns {
		if strings.Contains(msg, pattern) {
			observation.Who = "user"
			break
		}
	}

	// Extract "What" - the action/verb
	whatPatterns := []string{
		"create", "write", "read", "list", "show", "find",
		"search", "build", "make", "add", "edit", "modify",
		"delete", "remove", "calculate", "execute", "run",
	}
	for _, pattern := range whatPatterns {
		if idx := strings.Index(msg, pattern); idx != -1 {
			// Extract word after the verb as the "what"
			words := strings.Split(message[idx:], " ")
			if len(words) > 1 {
				observation.What = strings.Trim(words[1], ".,!?")
				break
			}
		}
	}

	// Extract "When" - time references
	whenPatterns := []string{"today", "now", "yesterday", "tomorrow", "before", "after", "next", "last"}
	for _, pattern := range whenPatterns {
		if strings.Contains(msg, pattern) {
			observation.When = pattern
			break
		}
	}

	// Extract "Where" - path/location references
	wherePatterns := []string{"/", "in ", "at ", "from ", "folder", "directory", "file"}
	for _, pattern := range wherePatterns {
		if strings.Contains(msg, pattern) {
			// Try to extract a path-like string
			words := strings.Fields(message)
			for _, w := range words {
				if strings.HasPrefix(w, "/") || strings.HasPrefix(w, "./") || strings.HasSuffix(w, ".go") || strings.HasSuffix(w, ".md") {
					observation.Where = w
					break
				}
			}
			break
		}
	}

	// Extract "Why" - purpose
	if strings.Contains(msg, "because") || strings.Contains(msg, "so that") || strings.Contains(msg, "to") {
		observation.Why = "user_requested"
	}

	// Extract "How" - method reference
	if strings.Contains(msg, "how") || strings.Contains(msg, "method") || strings.Contains(msg, "way") {
		observation.How = "to_be_determined"
	}

	// Build raw variables string for debugging
	var parts []string
	if observation.Who != "" {
		parts = append(parts, "who:"+observation.Who)
	}
	if observation.What != "" {
		parts = append(parts, "what:"+observation.What)
	}
	if observation.When != "" {
		parts = append(parts, "when:"+observation.When)
	}
	if observation.Where != "" {
		parts = append(parts, "where:"+observation.Where)
	}
	if observation.Why != "" {
		parts = append(parts, "why:"+observation.Why)
	}
	if observation.How != "" {
		parts = append(parts, "how:"+observation.How)
	}
	observation.RawVariables = strings.Join(parts, ", ")

	// Store entities for later use
	observation.Entities["task"] = intent.Task
	observation.Entities["confidence"] = fmt.Sprintf("%.0f", intent.Confidence*100)

	a.log.Debug("Observation: %s", observation.RawVariables)
	return observation
}

// STEP 2: HYPOTHESIS - Plan the experiments to fulfill the request
// This phase determines what tools to use based on the observation
func (a *Agent) hypothesize(observation Observation) Hypothesis {
	hypothesis := Hypothesis{
		Observation: observation,
		Confidence:  observation.Intent.Confidence,
	}

	// Form hypothesis text
	intent := observation.Intent
	hypothesis.HypothesisText = fmt.Sprintf("To fulfill the request '%s', I need to execute the following plan.", intent.Task)

	// Make predictions based on intent
	hypothesis.Predictions = a.makePredictions(intent)

	// Define success criteria as checklist
	hypothesis.SuccessCriteria = a.defineSuccessCriteria(intent)

	// Build experiment plan (will be overwritten by LLM in experiment phase)
	planStr := "to be determined by LLM in experiment phase"
	if len(hypothesis.ToolsNeeded) > 0 {
		planStr = strings.Join(hypothesis.ToolsNeeded, " -> ")
	}
	hypothesis.ExperimentPlan = fmt.Sprintf("Execute tools in sequence: %s", planStr)

	a.log.Debug("Hypothesis: %s", hypothesis.HypothesisText)
	a.log.Debug("Tools needed: %v", hypothesis.ToolsNeeded)
	return hypothesis
}

// STEP 3: EXPERIMENT - Execute the planned tools
// This phase runs tools to test our hypothesis
func (a *Agent) experiment(hypothesis Hypothesis) ExperimentResult {
	result := ExperimentResult{
		Hypothesis:   hypothesis,
		RawResults:   []string{},
		DataGathered: make(map[string]interface{}),
		Timestamp:    time.Now(),
	}

	// Determine if we should skip schema phase (for time-related queries)
	isTimeQuery := a.isTimeRelatedQuery(hypothesis.Observation.Message)
	ready := false

	if isTimeQuery {
		// Skip schema for time queries, go directly to tools
		a.log.Debug("Experiment: time-related query detected, using direct tool execution")
		ready = true
	} else {
		// Run schema phase to get structured plan from LLM
		schemaData := a.runSchemaPhase(hypothesis)
		result.DataGathered = schemaData.DataGathered
		ready = schemaData.Ready

		// Update hypothesis with schema-driven tools if available
		if len(schemaData.UpdatedTools) > 0 {
			hypothesis.ToolsNeeded = schemaData.UpdatedTools
		}
		if len(schemaData.UpdatedPredictions) > 0 {
			hypothesis.Predictions = schemaData.UpdatedPredictions
		}
		if len(schemaData.UpdatedCriteria) > 0 {
			hypothesis.SuccessCriteria = schemaData.UpdatedCriteria
		}
	}

	// Execute planned tools
	result = a.executePlannedTools(result, hypothesis)

	// Determine final ready state based on execution results
	result.Success = a.determineReadyState(result, hypothesis, ready)

	a.log.Debug("Experiment completed: %d tool executions in %v, ready=%v", len(result.ToolExecutions), result.Duration, result.Success)
	return result
}

// runSchemaPhase contacts the LLM to get a structured plan
// Returns schema data and whether we're ready to conclude
func (a *Agent) runSchemaPhase(hypothesis Hypothesis) SchemaPhaseResult {
	result := SchemaPhaseResult{
		DataGathered:       make(map[string]interface{}),
		UpdatedTools:       []string{},
		UpdatedPredictions: []string{},
		UpdatedCriteria:    []string{},
		Ready:              false,
	}

	// Force schema tool call FIRST to get structured plan
	// This enforces schema -> logic -> decision -> response
	inputJSON := fmt.Sprintf(`{"user_request": "%s"}`, hypothesis.Observation.Message)

	a.log.Debug("Experiment: calling scientific_method_plan to enforce schema")
	planResult, err := a.tools.Execute("scientific_method_plan", inputJSON, "")
	if err != nil {
		a.log.Error("Experiment: scientific_method_plan failed: %v", err)
		result.DataGathered["error"] = fmt.Sprintf("plan error: %v", err)
		return result
	}

	// Parse the returned JSON schema
	var planOutput map[string]interface{}
	if err := json.Unmarshal([]byte(planResult), &planOutput); err != nil {
		a.log.Error("Experiment: failed to parse plan result: %v", err)
		result.DataGathered["error"] = fmt.Sprintf("parse error: %v", err)
		return result
	}

	// Extract schema fields for logic
	step, _ := planOutput["step"].(string)
	intent, _ := planOutput["intent"].(string)
	ready, _ := planOutput["ready"].(bool)
	reasoning, _ := planOutput["reasoning"].(string)

	// Extract tools_needed from schema
	toolsNeededJSON, _ := json.Marshal(planOutput["tools_needed"])
	var toolsNeeded []string
	if err := json.Unmarshal(toolsNeededJSON, &toolsNeeded); err != nil {
		toolsNeeded = hypothesis.ToolsNeeded // Fallback to hypothesis tools
	} else {
		result.UpdatedTools = toolsNeeded
	}

	// Extract predictions from schema
	predictionsJSON, _ := json.Marshal(planOutput["predictions"])
	var predictions []string
	if err := json.Unmarshal(predictionsJSON, &predictions); err != nil {
		predictions = hypothesis.Predictions
	} else {
		result.UpdatedPredictions = predictions
	}

	// Extract success criteria from schema
	criteriaJSON, _ := json.Marshal(planOutput["success_criteria"])
	var successCriteria []string
	if err := json.Unmarshal(criteriaJSON, &successCriteria); err != nil {
		successCriteria = hypothesis.SuccessCriteria
	} else {
		result.UpdatedCriteria = successCriteria
	}

	// Store schema data in results
	result.DataGathered["step"] = step
	result.DataGathered["intent"] = intent
	result.DataGathered["ready"] = ready
	result.DataGathered["reasoning"] = reasoning
	result.DataGathered["tools_needed"] = toolsNeeded
	result.DataGathered["predictions"] = predictions
	result.DataGathered["success_criteria"] = successCriteria

	a.log.Debug("Experiment: schema plan - step=%s, intent=%s, ready=%v, reasoning=%s", step, intent, ready, reasoning)
	a.log.Debug("Experiment: tools_needed from schema: %v", toolsNeeded)

	// Use Ready field to determine next steps
	// If not Ready: continue experimenting (not ready to respond)
	// If Ready: proceed to conclude
	result.Ready = ready

	return result
}

// SchemaPhaseResult holds the results from running the schema phase
type SchemaPhaseResult struct {
	DataGathered       map[string]interface{}
	UpdatedTools       []string
	UpdatedPredictions []string
	UpdatedCriteria    []string
	Ready              bool
}

// executePlannedTools runs all tools in the hypothesis plan
func (a *Agent) executePlannedTools(result ExperimentResult, hypothesis Hypothesis) ExperimentResult {
	// Execute tools in sequence (using schema-driven tools)
	for _, toolName := range hypothesis.ToolsNeeded {
		toolExecution := ToolExecution{
			ToolName:  toolName,
			Timestamp: time.Now(),
		}

		// Execute the tool
		a.log.Debug("Experiment: executing tool %s", toolName)

		// Track that the experiment was planned
		toolExecution.Result = fmt.Sprintf("Planned execution of %s", toolName)
		toolExecution.Success = true

		result.ToolExecutions = append(result.ToolExecutions, toolExecution)
	}

	result.Duration = time.Now().Sub(result.Timestamp)
	return result
}

// determineReadyState checks if the experiment completed successfully
// Success is determined by: having executions OR intent is chat OR Ready=true from schema
func (a *Agent) determineReadyState(result ExperimentResult, hypothesis Hypothesis, schemaReady bool) bool {
	return len(result.ToolExecutions) > 0 || hypothesis.Observation.Intent.IsChat || schemaReady
}

// STEP 4: ANALYSIS - Compare results to predictions
// This phase evaluates whether the experiment succeeded
func (a *Agent) analyze(exp ExperimentResult, hypothesis Hypothesis) Analysis {
	analysis := Analysis{
		ExperimentResult: exp,
		Hypothesis:       hypothesis,
		ChecklistStatus:  make(map[string]bool),
	}

	// Initialize as not completed
	analysis.Completed = false

	// Check predictions met
	analysis.PredictionsMet = []string{}
	analysis.PredictionsFailed = []string{}

	for _, pred := range hypothesis.Predictions {
		// Simple check: if we have results, assume prediction could be met
		if len(exp.ToolExecutions) > 0 {
			analysis.PredictionsMet = append(analysis.PredictionsMet, pred)
		} else {
			analysis.PredictionsFailed = append(analysis.PredictionsFailed, pred)
		}
	}

	// Check success criteria
	analysis.TotalCriteria = len(hypothesis.SuccessCriteria)
	analysis.MetCriteria = 0

	for _, criterion := range hypothesis.SuccessCriteria {
		// Map intent flags to criteria
		analysis.ChecklistStatus[criterion] = a.evaluateCriterion(criterion, exp)

		if analysis.ChecklistStatus[criterion] {
			analysis.MetCriteria++
		}
	}

	// Determine if completed based on threshold
	gapThreshold := analysis.TotalCriteria / 2
	analysis.Completed = analysis.MetCriteria >= gapThreshold

	// Build gaps list
	analysis.Gaps = []string{}
	for criterion, met := range analysis.ChecklistStatus {
		if !met {
			analysis.Gaps = append(analysis.Gaps, criterion)
		}
	}

	// Build learnings
	analysis.Learnings = []string{}
	if analysis.Completed {
		analysis.Learnings = append(analysis.Learnings, fmt.Sprintf("Successfully completed %d/%d criteria", analysis.MetCriteria, analysis.TotalCriteria))
	} else {
		analysis.Learnings = append(analysis.Learnings, fmt.Sprintf("Incomplete: %d/%d criteria met", analysis.MetCriteria, analysis.TotalCriteria))
	}
	if len(exp.ToolExecutions) > 0 {
		analysis.Learnings = append(analysis.Learnings, fmt.Sprintf("Executed %d tools", len(exp.ToolExecutions)))
	}
	analysis.Learnings = append(analysis.Learnings, fmt.Sprintf("Confidence was %.0f%%", hypothesis.Confidence*100))

	a.log.Debug("Analysis: completed=%v, gaps=%v, learnings=%v", analysis.Completed, analysis.Gaps, analysis.Learnings)
	return analysis
}

// evaluateCriterion checks if a single success criterion was met
func (a *Agent) evaluateCriterion(criterion string, exp ExperimentResult) bool {
	switch criterion {
	case ChecklistInfoGathered:
		return exp.Hypothesis.Observation.Intent.NeedsInfo == false
	case ChecklistFilesRead:
		return exp.Hypothesis.Observation.Intent.NeedsFile == false
	case ChecklistComputed:
		return exp.Hypothesis.Observation.Intent.NeedsCompute == false
	case ChecklistWritten:
		return exp.Hypothesis.Observation.Intent.NeedsCreate == false
	case ChecklistResponseGen:
		return exp.Hypothesis.Observation.Intent.IsChat
	case ChecklistNoErrors:
		return exp.ErrorMessage == ""
	case ChecklistTaskCompleted:
		return exp.Success
	default:
		return len(exp.ToolExecutions) > 0
	}
}

// STEP 5: CONCLUSION - Form response from results
// This phase creates the final response based on experiment results
func (a *Agent) conclude(analysis Analysis, exp ExperimentResult) Conclusion {
	conclusion := Conclusion{
		Success:          analysis.Completed,
		NeedsImprovement: false,
	}

	// If experiment failed, mark for improvement
	if !analysis.Completed && len(analysis.Gaps) > 0 {
		conclusion.NeedsImprovement = true
		conclusion.Suggestion = fmt.Sprintf("Improve by addressing gaps: %s", strings.Join(analysis.Gaps, ", "))
	}

	// Build summary
	summaryParts := []string{}
	if analysis.Completed {
		summaryParts = append(summaryParts, "Task completed successfully")
	} else {
		summaryParts = append(summaryParts, "Task partially completed")
	}
	summaryParts = append(summaryParts, fmt.Sprintf("(%d/%d criteria met)", analysis.MetCriteria, analysis.TotalCriteria))
	conclusion.Summary = strings.Join(summaryParts, " ")

	// Response will be generated by runToolLoop, so we set a placeholder
	// The actual response comes from the LLM
	conclusion.Response = "experiment_completed"

	a.log.Debug("Conclusion: success=%v, needsImprovement=%v", conclusion.Success, conclusion.NeedsImprovement)
	return conclusion
}

// STEP 6: FEEDBACK LOOP - Record experiment and trigger improvements
// This phase learns from the experiment to improve future performance
func (a *Agent) feedback(analysis Analysis, hypothesis Hypothesis) {
	// Record experiment data to memory
	a.recordExperiment(analysis, hypothesis)

	// Check if improvement is needed
	if analysis.Completed {
		a.log.Debug("Feedback: experiment successful, no improvement needed")
		return
	}

	// Calculate failure rate
	failureRate := float64(len(analysis.Gaps)) / float64(analysis.TotalCriteria)

	// If failure rate exceeds threshold, trigger self-improvement
	if failureRate > 0.3 && a.detector != nil {
		a.log.Info("Feedback: triggering self-improvement due to high failure rate (%.0f%%)", failureRate*100)

		// Record gaps for self-improvement
		for _, gap := range analysis.Gaps {
			a.detector.RecordError("execution", gap, "experiment")
		}
	}

	// Update MEMORY.md with learnings
	a.updateMemory(analysis, hypothesis)
}

// =============================================================================
// INTENT ANALYSIS
// =============================================================================

// analyzeIntent examines user message and returns detected intent
// Uses LLM inference via scientific_method_plan instead of Go patterns
func (a *Agent) analyzeIntent(userMessage string) Intent {
	// Use LLM to determine intent instead of Go patterns
	// The LLM in scientific_method_plan will decide what tools are needed
	// Don't set any defaults - let the LLM decide

	intent := Intent{
		Task:       userMessage,
		Confidence: 0.5,
	}

	// Analyze message for intent flags
	msg := strings.ToLower(userMessage)

	// Determine if needs external info
	if strings.Contains(msg, "search") || strings.Contains(msg, "find") ||
		strings.Contains(msg, "look up") || strings.Contains(msg, "what is") {
		intent.NeedsInfo = true
	}

	// Determine if needs file access
	if strings.Contains(msg, "read") || strings.Contains(msg, "file") ||
		strings.Contains(msg, "show") || strings.Contains(msg, "list") {
		intent.NeedsFile = true
	}

	// Determine if needs computation
	if strings.Contains(msg, "calculate") || strings.Contains(msg, "compute") ||
		strings.Contains(msg, "run") || strings.Contains(msg, "execute") ||
		strings.Contains(msg, "script") {
		intent.NeedsCompute = true
	}

	// Determine if needs to create/write
	if strings.Contains(msg, "create") || strings.Contains(msg, "write") ||
		strings.Contains(msg, "make") || strings.Contains(msg, "add") {
		intent.NeedsCreate = true
	}

	// Determine if just conversational
	if strings.Contains(msg, "hello") || strings.Contains(msg, "hi") ||
		strings.Contains(msg, "how are") || strings.Contains(msg, "thanks") ||
		strings.Contains(msg, "thank you") {
		intent.IsChat = true
	}

	// Set required tools based on flags
	if intent.NeedsInfo {
		intent.RequiredTools = append(intent.RequiredTools, "memory_read")
	}
	if intent.NeedsFile {
		intent.RequiredTools = append(intent.RequiredTools, "file_read", "dir_list")
	}
	if intent.NeedsCompute {
		intent.RequiredTools = append(intent.RequiredTools, "lua_exec")
	}
	if intent.NeedsCreate {
		intent.RequiredTools = append(intent.RequiredTools, "file_write", "git_commit")
	}

	return intent
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// isTimeRelatedQuery checks if the message is about time/date
func (a *Agent) isTimeRelatedQuery(message string) bool {
	msgLower := strings.ToLower(message)
	for _, keyword := range timeQueryKeywords {
		if strings.Contains(msgLower, keyword) {
			return true
		}
	}
	return false
}

// determineToolsNeeded maps intent to required tools
func (a *Agent) determineToolsNeeded(intent Intent) []string {
	toolsNeeded := []string{}

	// Add tools from intent
	toolsNeeded = append(toolsNeeded, intent.RequiredTools...)

	// Add default tools based on intent type
	if intent.NeedsInfo {
		toolsNeeded = append(toolsNeeded, "memory_read")
	}
	if intent.NeedsFile {
		toolsNeeded = append(toolsNeeded, "file_read", "dir_list")
	}
	if intent.NeedsCompute {
		toolsNeeded = append(toolsNeeded, "lua_exec")
	}
	if intent.NeedsCreate {
		toolsNeeded = append(toolsNeeded, "file_write", "git_commit")
	}
	// No tools needed for conversation (intent.IsChat)

	// Deduplicate
	seen := make(map[string]bool)
	uniqueTools := []string{}
	for _, tool := range toolsNeeded {
		if !seen[tool] {
			seen[tool] = true
			uniqueTools = append(uniqueTools, tool)
		}
	}

	return uniqueTools
}

// makePredictions returns predicted outcomes based on intent
func (a *Agent) makePredictions(intent Intent) []string {
	predictions := []string{}

	switch intent.Task {
	case TaskInfoGathering:
		predictions = append(predictions, "External information will be retrieved", "Relevant data will be found")
	case TaskFileAccess:
		predictions = append(predictions, "Files will be read successfully", "Content will be displayed")
	case TaskComputation:
		predictions = append(predictions, "Computation will execute", "Result will be returned")
	case TaskFileCreation:
		predictions = append(predictions, "Files will be created/modified", "Changes will be saved")
	case TaskConversation:
		predictions = append(predictions, "Response will be generated")
	default:
		predictions = append(predictions, "Task will complete successfully")
	}

	return predictions
}

// defineSuccessCriteria returns checklist items for success
func (a *Agent) defineSuccessCriteria(intent Intent) []string {
	criteria := []string{}

	if intent.NeedsInfo {
		criteria = append(criteria, ChecklistInfoGathered, ChecklistExternalData)
	}
	if intent.NeedsFile {
		criteria = append(criteria, ChecklistFilesRead, ChecklistContentRetrieved)
	}
	if intent.NeedsCompute {
		criteria = append(criteria, ChecklistComputed, ChecklistResultGenerated)
	}
	if intent.NeedsCreate {
		criteria = append(criteria, ChecklistWritten, ChecklistFilesSaved)
	}
	if intent.IsChat {
		criteria = append(criteria, ChecklistResponseGen)
	}

	// Add general criteria
	criteria = append(criteria, ChecklistNoErrors, ChecklistTaskCompleted)

	return criteria
}

// verifyChecklist checks if required intent items have been completed
func (a *Agent) verifyChecklist(checklist map[string]bool, intent Intent) string {
	var missing []string

	// Map intent flags to checklist items
	if intent.NeedsInfo && !checklist[ChecklistInfoGathered] {
		missing = append(missing, "external information gathering")
	}
	if intent.NeedsFile && !checklist[ChecklistFilesRead] {
		missing = append(missing, "file access")
	}
	if intent.NeedsCompute && !checklist[ChecklistComputed] {
		missing = append(missing, "computation")
	}
	if intent.NeedsCreate && !checklist[ChecklistWritten] {
		missing = append(missing, "file creation/modification")
	}

	if len(missing) > 0 {
		return "Incomplete: " + strings.Join(missing, ", ")
	}
	return ""
}

// buildSystemPromptWithIntent builds system prompt with intent-aware guidance
func (a *Agent) buildSystemPromptWithIntent(intent Intent, inferredContext string) (string, error) {
	basePrompt, err := a.buildSystemPrompt()
	if err != nil {
		return "", err
	}

	// Add intent-specific guidance
	intentGuidance := "\n## Current Task Analysis\n"
	intentGuidance += fmt.Sprintf("Detected task: %s (confidence: %.0f%%)\n", intent.Task, intent.Confidence*100)

	// Add inferred context if available
	if inferredContext != "" {
		intentGuidance += "\n## Relevant Past Context\n" + inferredContext + "\n"
	}

	if intent.NeedsInfo {
		intentGuidance += "- Priority: Gather external information first\n"
	}
	if intent.NeedsFile {
		intentGuidance += "- Priority: Read relevant files before proceeding\n"
	}
	if intent.NeedsCompute {
		intentGuidance += "- Priority: Execute computation/logic\n"
	}
	if intent.NeedsCreate {
		intentGuidance += "- Priority: Create or modify files as needed\n"
	}
	if intent.IsChat {
		intentGuidance += "- Priority: Conversational response\n"
	}

	if len(intent.RequiredTools) > 0 {
		intentGuidance += fmt.Sprintf("- Recommended tools: %s\n", strings.Join(intent.RequiredTools, ", "))
	}

	return basePrompt + intentGuidance, nil
}

// =============================================================================
// TOOL LOOP
// =============================================================================

// runToolLoop orchestrates the back-and-forth communication with the LLM
// Handles sending messages, processing tool calls, and managing the iteration cycle
func (a *Agent) runToolLoop(systemPrompt string, sess *session.Session) (string, error) {
	// Only send recent messages (last N) to execution LLM to keep context focused
	recentMsgs := getRecentMessages(sess.Messages, maxHistoryMessages)
	messages := a.sessionToLLM(recentMsgs)
	nudgeCount := 0

	// DEBUG: Log loop start
	a.log.Debug("=== TOOL LOOP STARTING ===")
	a.log.Debug("Initial messages: %d", len(messages))

	for i := 0; i < maxToolIterations; i++ {
		a.log.Debug("=== Tool Loop Iteration %d ===", i)

		// Send message to LLM and get response
		resp, toolCalls, err := a.sendToLLM(systemPrompt, messages)
		if err != nil {
			return resp, err
		}

		// No tool calls made - add a nudging message and retry
		// This forces the LLM to use a tool next time
		if len(toolCalls) == 0 {
			nudgeCount++
			if nudgeCount >= maxNudges {
				// Too many nudges - give up and return a message
				a.log.Warn("Too many nudges (%d), giving up", nudgeCount)
				return "I apologize but I am having trouble completing this request. Please try again.", nil
			}
			a.log.Debug("NO TOOL CALLS - adding nudge %d/%d and retrying", nudgeCount, maxNudges)

			// Add a nudging message to the conversation
			nudgeMsg := llm.Message{
				Role:    "user",
				Content: "Please use a tool to complete this request. Use message tool to respond to the user once you have the information.",
			}
			messages = append(messages, nudgeMsg)

			// Continue to next iteration - this will retry with the nudge
			continue
		}

		a.log.Debug("Tool calls: %d", len(toolCalls))

		// Add assistant message to conversation
		messages = append(messages, llm.Message{
			Role:    "assistant",
			Content: resp,
		})

		// Process all tool calls
		messages = a.processToolCalls(toolCalls, messages, sess.ID)

		// Exit loop if message tool was called - this is the final response
		for _, tc := range toolCalls {
			if tc.Function.Name == "message" {
				a.log.Debug("Message tool called - getting tool result")
				// Find the corresponding tool result in messages
				// It was added by processToolCalls - get the last message result
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == "tool" && messages[i].Name == "message" {
						a.log.Debug("Message tool result: %s", messages[i].Content)
						return messages[i].Content, nil
					}
				}
				// Fallback if no tool result found
				return resp, nil
			}
		}

		// Reset nudge count on successful tool execution
		nudgeCount = 0
	}

	return "Maximum tool iterations reached", nil
}

// sendToLLM sends messages to the LLM and returns the response with any tool calls
func (a *Agent) sendToLLM(systemPrompt string, messages []llm.Message) (string, []llm.ToolCall, error) {
	// NOTE: Tools are sent internally to LLM via getTools() in client
	a.log.Debug("Sending to LLM: %d messages (tools sent internally)", len(messages))
	a.log.Debug("LLM REQUEST: calling Chat with %d messages", len(messages))

	resp, toolCalls, err := a.llmCli.Chat(systemPrompt, messages)

	// DEBUG: Log LLM response received
	a.log.Debug("LLM RESPONSE: has_content=%v, has_toolcalls=%v, error=%v", resp != "", len(toolCalls) > 0, err)

	// DEBUG: Log actual response content
	if len(resp) > 0 {
		a.log.Debug("LLM RESPONSE CONTENT: %s", resp)
	}

	// DEBUG: Log each tool call detected
	for _, toolCall := range toolCalls {
		a.log.Debug("TOOL CALL DETECTED: %s with args %s", toolCall.Function.Name, string(toolCall.Function.Arguments))
	}

	return resp, toolCalls, err
}

// processToolCalls executes each tool call and adds results to the message list
func (a *Agent) processToolCalls(toolCalls []llm.ToolCall, messages []llm.Message, sessionID string) []llm.Message {
	for _, toolCall := range toolCalls {
		result := a.handleToolResult(toolCall, sessionID)

		// Add tool result to conversation
		toolMsg := llm.Message{
			Role:    "tool",
			Content: result,
			Name:    toolCall.Function.Name,
		}
		messages = append(messages, toolMsg)
		a.log.Debug("Fed %d bytes back to LLM as tool result", len(result))
	}
	return messages
}

// handleToolResult executes a single tool call and returns the result
// Also handles error tracking and logging
func (a *Agent) handleToolResult(toolCall llm.ToolCall, sessionID string) string {
	toolName := toolCall.Function.Name

	// DEBUG: Log before execution
	a.log.Debug("EXECUTING TOOL: %s", toolName)

	result, err := a.executeTool(toolCall)

	// DEBUG: Log execution result
	a.log.Debug("TOOL RESULT for %s: error=%v, result_length=%d", toolName, err, len(result))

	if err != nil {
		a.log.Error("Tool error %s: %v", toolName, err)
		result = fmt.Sprintf("Error: %v", err)

		if a.detector != nil {
			a.detector.RecordError(toolName, err.Error(), sessionID)
		}
	} else {
		a.log.Debug("Tool %s result: %s", toolName, result)
	}

	return result
}

// =============================================================================
// MEMORY AND RECORDING
// =============================================================================

// recordExperiment records experiment data to memory
func (a *Agent) recordExperiment(analysis Analysis, hypothesis Hypothesis) {
	if a.memory == nil {
		return
	}

	// Create experiment record
	record := map[string]interface{}{
		"experiment_id":  fmt.Sprintf("exp_%d", time.Now().Unix()),
		"hypothesis":     hypothesis.HypothesisText,
		"completed":      analysis.Completed,
		"met_criteria":   analysis.MetCriteria,
		"total_criteria": analysis.TotalCriteria,
		"confidence":     hypothesis.Confidence,
		"learnings":      analysis.Learnings,
		"timestamp":      time.Now().Format(time.RFC3339),
	}

	// Store in memory using AppendToToday
	jsonRecord, err := json.Marshal(record)
	if err != nil {
		a.log.Error("Failed to marshal experiment record: %v", err)
		return
	}

	entry := fmt.Sprintf("experiment: %s", string(jsonRecord))
	if err := a.memory.AppendToToday(entry); err != nil {
		a.log.Error("Failed to append experiment to memory: %v", err)
	}
}

// updateMemory updates MEMORY.md with learnings
func (a *Agent) updateMemory(analysis Analysis, hypothesis Hypothesis) {
	if a.memory == nil {
		return
	}

	// Add learnings to today's memory
	for _, learning := range analysis.Learnings {
		entry := fmt.Sprintf("learning: %s", learning)
		if err := a.memory.AppendToToday(entry); err != nil {
			a.log.Error("Failed to append learning to memory: %v", err)
		}
	}

	a.log.Debug("Memory updated with %d learnings", len(analysis.Learnings))
}

// =============================================================================
// SELF-IMPROVEMENT
// =============================================================================

// checkForSelfImprovement examines response for self-improvement opportunities
func (a *Agent) checkForSelfImprovement(response, sessionID string) {
	if a.detector == nil || a.tools == nil {
		return
	}

	gapErrors, err := a.detector.GetErrors()
	if err != nil {
		return
	}

	for tool, rec := range gapErrors {
		if suggestion, ok := a.detector.GetGap(tool); ok && a.cfg.SelfImprove.AutoCreateSkills {
			a.log.Info("Auto-creating skill for gap: %s", suggestion)
			if err := a.tools.CreateSkillFromGap(tool, suggestion); err != nil {
				a.log.Error("Failed to create skill: %v", err)
				return
			}

			if a.gitMgr != nil {
				a.gitMgr.CommitSkillChange(tool, "auto-created from gap")
			}
		}
		_ = rec
	}
}

// =============================================================================
// TOOL EXECUTION
// =============================================================================

// executeTool runs a tool with the given arguments
func (a *Agent) executeTool(toolCall llm.ToolCall) (string, error) {
	name := toolCall.Function.Name
	args := toolCall.Function.Arguments

	return a.tools.Execute(name, string(args), toolCall.ID)
}

// =============================================================================
// SESSION HELPERS
// =============================================================================

// sessionToLLM converts session messages to LLM message format
func (a *Agent) sessionToLLM(sessMsgs []session.Message) []llm.Message {
	var result []llm.Message
	for _, m := range sessMsgs {
		result = append(result, llm.Message{
			Role:    m.Role,
			Content: m.Content,
			Name:    m.Name,
		})
	}
	return result
}

// getRecentMessages returns only the last n messages from the session
// This limits what gets sent to the execution LLM for focused context
func getRecentMessages(msgs []session.Message, n int) []session.Message {
	if len(msgs) <= n {
		return msgs
	}
	return msgs[len(msgs)-n:]
}

// buildSystemPrompt constructs the base system prompt from bootstrap files
func (a *Agent) buildSystemPrompt() (string, error) {
	files, err := a.ws.GetBootstrapFiles(a.cfg.Agent.InjectMode)
	if err != nil {
		return "", err
	}

	prompt := ""
	for _, f := range files {
		prompt += string(f.Content) + "\n\n"
	}

	skillList, err := a.skills.List()
	if err == nil && len(skillList) > 0 {
		prompt += "\n## Available Skills\n"
		for _, name := range skillList {
			skill, err := a.skills.Load(name)
			if err == nil {
				prompt += fmt.Sprintf("- %s: %s\n", name, skill.Description)
			}
		}
	}

	toolDocs := a.tools.GetToolDocs()
	prompt += "\n" + toolDocs

	// Add explicit instruction about tool calling
	prompt += "\n\n## Tool Calling Instructions\n"
	prompt += "You have 3 tools available:\n\n"
	prompt += "1. **lua_exec** — Your PRIMARY tool for ALL world interaction.\n"
	prompt += "   Use this to execute Lua code. The sandbox provides these modules:\n"
	prompt += "   - file.read/write/edit/list — File operations\n"
	prompt += "   - web.fetch/search — Web page fetching and searching\n"
	prompt += "   - browser.navigate/click/type/screenshot — Browser automation\n"
	prompt += "   - memory.today/get/write/search — Memory management\n"
	prompt += "   - skill.list/exec/create — Skill management\n"
	prompt += "   - scheduler.add/remove/list — Scheduled tasks\n"
	prompt += "   - os.date/time, time.time/date — Time utilities\n"
	prompt += "   - print() — Capture intermediate output\n"
	prompt += "   RULE: If you need to interact with files, the web, memory, or any external system, do it through lua_exec.\n\n"
	prompt += "2. **message** — Call ONLY when you have the final response for the user.\n"
	prompt += "   Do NOT call message for intermediate steps. Use print() in lua_exec to see results.\n\n"
	prompt += "3. **scientific_method_plan** — Call for structured planning and analysis.\n"
	prompt += "   Generates a JSON plan with tools_needed, predictions, and success_criteria.\n\n"
	prompt += "Tool calls are handled via the API's tool_calls parameter. Return the function name and arguments."

	return prompt, nil
}


