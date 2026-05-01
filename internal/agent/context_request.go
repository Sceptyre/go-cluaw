package agent

// ContextRequest represents the structured output from the inference LLM
type ContextRequest struct {
	// Query types define what kind of context to retrieve
	QueryTypes []string `json:"query_types"` // e.g., "conversation", "file_operations", "errors"`

	// Keywords for semantic search
	Keywords []string `json:"keywords"`

	// Specific references (e.g., file paths, session IDs)
	References []string `json:"references"`

	// Time range hint (e.g., "recent", "today", "this week")
	TimeRange string `json:"time_range"`

	// Confidence that context is needed (0.0 - 1.0)
	Confidence float64 `json:"confidence"`

	// Specific questions to answer from context
	Questions []string `json:"questions"`
}

// ContextResponse contains the retrieved context to inject into the execution prompt
type ContextResponse struct {
	// Retrieved context items
	Items []ContextItem `json:"items"`

	// Summary of what was found
	Summary string `json:"summary"`

	// Whether any context was found
	HasContext bool `json:"has_context"`
}

// ContextItem represents a single piece of retrieved context
type ContextItem struct {
	Type      string  `json:"type"`      // "message", "file_op", "error", "preference"
	Content   string  `json:"content"`   // The actual context content
	Source    string  `json:"source"`    // Where it came from (session ID, file path)
	Timestamp string  `json:"timestamp"` // When it occurred
	Relevance float64 `json:"relevance"` // How relevant (0.0 - 1.0)
}
