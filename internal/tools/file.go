package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileTool struct {
	workspace string
}

func NewFileTool(workspace string) *FileTool {
	return &FileTool{workspace: workspace}
}

func (t *FileTool) Read(path string) (string, error) {
	fullPath := t.resolvePath(path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}

func (t *FileTool) Write(path, content string) (string, error) {
	fullPath := t.resolvePath(path)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("OK: written %d bytes to %s", len(content), path), nil
}

func (t *FileTool) Edit(path, search, replace string) (string, error) {
	fullPath := t.resolvePath(path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	newContent := strings.Replace(content, search, replace, 1)
	if newContent == content {
		return "", fmt.Errorf("search string not found")
	}

	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("Edited %s", path), nil
}

func (t *FileTool) List(dir string) ([]string, error) {
	fullPath := t.resolvePath(dir)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		files = append(files, e.Name())
	}
	return files, nil
}

func (t *FileTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.workspace, path)
}

func (t *FileTool) IsAllowed(path string) bool {
	resolved := t.resolvePath(path)
	return strings.HasPrefix(resolved, t.workspace)
}
