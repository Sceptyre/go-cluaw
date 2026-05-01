package self_improve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Detector struct {
	workspace string
	errorFile string
	threshold int
}

type ErrorRecord struct {
	Tool       string    `json:"tool"`
	Error      string    `json:"error"`
	Count      int       `json:"count"`
	LastSeen   time.Time `json:"last_seen"`
	SessionID  string    `json:"session_id"`
	Suggestion string    `json:"suggestion,omitempty"`
}

func NewDetector(workspace string, threshold int) *Detector {
	return &Detector{
		workspace: workspace,
		errorFile: filepath.Join(workspace, "sessions", "errors.json"),
		threshold: threshold,
	}
}

func (d *Detector) RecordError(tool, errorMsg, sessionID string) error {
	record := d.loadRecord(tool)

	record.Count++
	record.LastSeen = time.Now()
	record.SessionID = sessionID
	record.Error = errorMsg

	if record.Count >= d.threshold {
		record.Suggestion = d.analyzeGap(tool, errorMsg)
	}

	return d.saveRecord(tool, record)
}

func (d *Detector) GetErrors() (map[string]ErrorRecord, error) {
	errorsDir := filepath.Join(d.workspace, "sessions", "errors")

	if _, err := os.Stat(errorsDir); os.IsNotExist(err) {
		return make(map[string]ErrorRecord), nil
	}

	entries, err := os.ReadDir(errorsDir)
	if err != nil {
		return nil, err
	}

	result := make(map[string]ErrorRecord)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		name := e.Name()
		if len(name) < 5 || name[len(name)-5:] != ".json" {
			continue
		}

		tool := name[:len(name)-5]
		path := filepath.Join(errorsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var record ErrorRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		result[tool] = record
	}

	return result, nil
}

func (d *Detector) GetGap(tool string) (string, bool) {
	record := d.loadRecord(tool)
	if record.Count >= d.threshold && record.Suggestion != "" {
		return record.Suggestion, true
	}
	return "", false
}

func (d *Detector) ClearErrors(tool string) error {
	errFile := d.errorFileFor(tool)
	return os.Remove(errFile)
}

func (d *Detector) loadRecord(tool string) ErrorRecord {
	errFile := d.errorFileFor(tool)
	data, err := os.ReadFile(errFile)
	if err != nil {
		return ErrorRecord{Tool: tool}
	}

	var record ErrorRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return ErrorRecord{Tool: tool}
	}

	return record
}

func (d *Detector) saveRecord(tool string, record ErrorRecord) error {
	errDir := filepath.Join(d.workspace, "sessions", "errors")
	if err := os.MkdirAll(errDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(d.errorFileFor(tool), data, 0644)
}

func (d *Detector) errorFileFor(tool string) string {
	if tool == "" {
		return d.errorFile
	}
	return filepath.Join(d.workspace, "sessions", "errors", tool+".json")
}

func (d *Detector) analyzeGap(tool, errorMsg string) string {
	suggestions := map[string]string{
		"unknown_tool":     "Create a skill to handle unknown commands",
		"file_not_found":   "Create skill for better file discovery",
		"web_fetch_failed": "Add fallback web fetch capability",
		"lua_error":        "Improve Lua execution robustness",
		"browser_failed":   "Add headless browser alternatives",
	}

	for key, suggestion := range suggestions {
		if containsStr(errorMsg, key) {
			return suggestion
		}
	}

	return fmt.Sprintf("Consider creating a skill to improve %s handling", tool)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

var _ = fmt.Println
var _ = time.Now
