package gitops

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Manager struct {
	workspace      string
	autoCommit     bool
	errorThreshold int
	commitPrefix   string
}

type ErrorRecord struct {
	Skill    string    `json:"skill"`
	Error    string    `json:"error"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
}

func New(workspace string, autoCommit bool, errorThreshold int, commitPrefix string) *Manager {
	return &Manager{
		workspace:      workspace,
		autoCommit:     autoCommit,
		errorThreshold: errorThreshold,
		commitPrefix:   commitPrefix,
	}
}

func (m *Manager) Init() error {
	gitDir := m.workspace + "/.git"
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := m.runGit("init"); err != nil {
			return fmt.Errorf("git init: %w", err)
		}
	}

	if err := os.MkdirAll(m.workspace+"/sessions", 0755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}

	if err := os.MkdirAll(m.workspace+"/sessions/errors", 0755); err != nil {
		return fmt.Errorf("create errors dir: %w", err)
	}

	return nil
}

func (m *Manager) CommitSkillChange(skillName, message string) error {
	if !m.autoCommit {
		return nil
	}

	skillPath := filepath.Join(m.workspace, "skills", skillName)
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		return fmt.Errorf("skill not found: %s", skillName)
	}

	if err := m.runGit("add", "skills/"+skillName); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	commitMsg := m.commitPrefix + skillName + " - " + message
	if err := m.runGit("commit", "-m", commitMsg); err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit: %w", err)
	}

	return nil
}

func (m *Manager) RecordError(skillName, errMsg string) error {
	record := m.loadErrorRecord(skillName)
	record.Count++
	record.LastSeen = time.Now()

	if record.Count >= m.errorThreshold {
		if err := m.RevertLastCommit(); err != nil {
			return fmt.Errorf("auto-revert failed: %w", err)
		}
		record.Count = 0
	}

	return m.saveErrorRecord(skillName, errMsg, record.Count)
}

func (m *Manager) RevertLastCommit() error {
	if err := m.runGit("revert", "HEAD"); err != nil {
		return fmt.Errorf("git revert: %w", err)
	}
	return nil
}

func (m *Manager) GetHistory(skillName string) ([]string, error) {
	out, err := m.runGitOutput("log", "--oneline", "--all", "--", "skills/"+skillName)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(out) == "" {
		return []string{}, nil
	}

	return strings.Split(out, "\n"), nil
}

func (m *Manager) runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.workspace
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *Manager) runGitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.workspace
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (m *Manager) loadErrorRecord(skillName string) *ErrorRecord {
	path := m.errorFilePath(skillName)
	data, err := os.ReadFile(path)
	if err != nil {
		return &ErrorRecord{Skill: skillName}
	}

	var record ErrorRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return &ErrorRecord{Skill: skillName}
	}

	return &record
}

func (m *Manager) saveErrorRecord(skillName, errMsg string, count int) error {
	path := m.errorFilePath(skillName)
	record := ErrorRecord{
		Skill:    skillName,
		Error:    errMsg,
		Count:    count,
		LastSeen: time.Now(),
	}

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (m *Manager) errorFilePath(skillName string) string {
	return m.workspace + "/sessions/errors/" + skillName + ".json"
}
