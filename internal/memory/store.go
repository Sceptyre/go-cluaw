package memory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Store struct {
	workspace string
}

func NewStore(workspace string) *Store {
	return &Store{workspace: workspace}
}

func (s *Store) GetTodayPath() string {
	now := time.Now()
	return now.Format("2006-01-02.md")
}

func (s *Store) EnsureTodayExists() error {
	todayFile := s.GetTodayPath()
	path := filepath.Join(s.workspace, "memory", todayFile)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		now := time.Now()
		content := fmt.Sprintf(`# %s

## Session
- started: %s

## Notes


`, now.Format("2006-01-02"), now.Format("15:04"))

		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}

	return s.EnsureIndexExists()
}

func (s *Store) EnsureIndexExists() error {
	indexPath := filepath.Join(s.workspace, "memory", "index.md")

	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		content := `# Memory Index

## By Date

## By Tag

`
		if err := os.WriteFile(indexPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) GetTodayContent() (string, error) {
	todayFile := s.GetTodayPath()
	path := filepath.Join(s.workspace, "memory", todayFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (s *Store) AppendToToday(entry string) error {
	todayFile := s.GetTodayPath()
	path := filepath.Join(s.workspace, "memory", todayFile)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	now := time.Now()
	markedEntry := fmt.Sprintf("- %s: %s\n", now.Format("15:04"), entry)
	_, err = f.WriteString(markedEntry)
	return err
}

func (s *Store) GetFile(filename string) (string, error) {
	path := filepath.Join(s.workspace, "memory", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Store) WriteFile(filename, content string) error {
	path := filepath.Join(s.workspace, "memory", filename)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// Search searches the memory store for entries matching the query
func (s *Store) Search(query string) ([]string, error) {
	memoryDir := filepath.Join(s.workspace, "memory")

	if _, err := os.Stat(memoryDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	// Use grep to find matching entries
	cmd := exec.Command("grep", "-r", "-i", "--include=*.md", "-h", query, memoryDir)
	output, err := cmd.Output()
	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			return []string{}, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var results []string
	for _, line := range lines {
		if line != "" {
			results = append(results, line)
		}
	}

	return results, nil
}
