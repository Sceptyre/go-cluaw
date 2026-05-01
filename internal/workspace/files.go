package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Workspace struct {
	path  string
	files map[string][]byte
}

type BootstrapFile struct {
	Name    string
	Path    string
	Content []byte
}

func New(path string) (*Workspace, error) {
	expanded := expandPath(path)

	if err := os.MkdirAll(expanded, 0755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	if err := os.MkdirAll(expanded+"/skills", 0755); err != nil {
		return nil, fmt.Errorf("create skills dir: %w", err)
	}

	if err := os.MkdirAll(expanded+"/memory", 0755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	if err := os.MkdirAll(expanded+"/sessions", 0755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	w := &Workspace{
		path:  expanded,
		files: make(map[string][]byte),
	}

	if err := w.ensureBootstrapFiles(); err != nil {
		return nil, err
	}

	return w, nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = home + path[1:]
	}
	return path
}

func (w *Workspace) ensureBootstrapFiles() error {
	bootstrapFiles := []string{
		"AGENTS.md",
		"SOUL.md",
		"USER.md",
		"MEMORY.md",
		"TOOLS.md",
	}

	for _, name := range bootstrapFiles {
		path := filepath.Join(w.path, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			content := getDefaultContent(name)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("write %s: %w", name, err)
			}
		}
	}

	return nil
}

func getDefaultContent(name string) string {
	switch name {
	case "AGENTS.md":
		return `# AGENTS.md

You are cluaw, an autonomous AI agent.

## Operating Rules
- Always complete tasks you start
- If you need additional capabilities, create them as skills
- Log important learnings to MEMORY.md

## Workflow
1. Understand the user's request
2. Plan your approach
3. Execute with available tools
4. If you lack capability, create it
5. Report results

## Tool Priority

When the user asks for something that requires action (calculations, file operations, web fetching, etc), use lua_exec FIRST before using other tools.

- **lua_exec** is the primary means of doing something outside of speaking
- Other tools (file, web_fetch, etc) are secondary
`
	case "SOUL.md":
		return `# SOUL.md

I am cluaw, an AI assistant that thinks and acts autonomously.

## Communication
- Clear and concise
- Explain my reasoning when helpful
- Ask clarifying questions when needed

## Values
- Be helpful and practical
- Admit when I don't know something
- Continuously improve my capabilities
`
	case "USER.md":
		return `# USER.md

User context will be added here.

[To be filled by the user]
`
	case "MEMORY.md":
		return `# MEMORY.md

Long-term memory and learnings.

[Written by agent over time]
`
	case "TOOLS.md":
		return `# TOOLS.md

## Available Tools

### file
- file.read(path) - Read a file
- file.write(path, content) - Write to a file
- file.list(dir) - List directory contents

### exec
- exec(script) - Execute Lua code

### message
- message(content) - Send a Discord message

### skill
- skill.list() - List available skills
- skill.exec(name, args) - Execute a skill
`
	default:
		return ""
	}
}

func (w *Workspace) Path() string {
	return w.path
}

func (w *Workspace) GetBootstrapFile(name string) (*BootstrapFile, error) {
	path := filepath.Join(w.path, name)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &BootstrapFile{
		Name:    name,
		Path:    path,
		Content: content,
	}, nil
}

func (w *Workspace) GetBootstrapFiles(mode string) ([]*BootstrapFile, error) {
	files := []string{
		"AGENTS.md",
		"SOUL.md",
	}

	if mode != "minimal" {
		files = append(files, "USER.md", "MEMORY.md")
	}

	if mode == "every-turn" {
		files = append(files, "TOOLS.md")
	}

	var result []*BootstrapFile
	for _, name := range files {
		f, err := w.GetBootstrapFile(name)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		result = append(result, f)
	}

	return result, nil
}

func (w *Workspace) ReadFile(path string) ([]byte, error) {
	fullPath := filepath.Join(w.path, path)
	return os.ReadFile(fullPath)
}

func (w *Workspace) WriteFile(path string, content []byte) error {
	fullPath := filepath.Join(w.path, path)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return os.WriteFile(fullPath, content, 0644)
}

func (w *Workspace) ListSkills() ([]string, error) {
	entries, err := os.ReadDir(w.path + "/skills")
	if err != nil {
		return nil, err
	}

	var skills []string
	for _, e := range entries {
		if e.IsDir() {
			skills = append(skills, e.Name())
		}
	}
	return skills, nil
}

func (w *Workspace) GetSkill(name string) ([]byte, error) {
	path := filepath.Join(w.path, "skills", name, "SKILL.md")
	return os.ReadFile(path)
}
