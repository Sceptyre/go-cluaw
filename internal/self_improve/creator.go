package self_improve

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Creator struct {
	workspace string
}

func NewCreator(workspace string) *Creator {
	return &Creator{workspace: workspace}
}

type SkillSpec struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	Runtime      string `json:"runtime,omitempty"`
	Timeout      int    `json:"timeout,omitempty"`
}

func (c *Creator) Create(skill SkillSpec) error {
	if skill.Name == "" {
		return fmt.Errorf("skill name required")
	}

	skillsDir := filepath.Join(c.workspace, "skills", skill.Name)
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return err
	}

	content := c.formatSkillFile(skill)
	skillPath := filepath.Join(skillsDir, "SKILL.md")

	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		return err
	}

	return c.CreateDefaultScripts(skill.Name)
}

func (c *Creator) formatSkillFile(skill SkillSpec) string {
	runtime := skill.Runtime
	if runtime == "" {
		runtime = "lua"
	}

	timeout := skill.Timeout
	if timeout == 0 {
		timeout = 30
	}

	return fmt.Sprintf(`---
name: %s
description: %s
version: "1.0.0"
author: "cluaw"
runtime: %s
timeout: %d
---

# %s

%s
`, skill.Name, skill.Description, runtime, timeout, skill.Name, skill.Instructions)
}

func (c *Creator) CreateDefaultScripts(skillName string) error {
	scriptsDir := filepath.Join(c.workspace, "skills", skillName, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return err
	}

	content := fmt.Sprintf(`-- %s skill
-- Created: %s

local function main(args)
    return "Skill executed: %s"
end

return main
`, skillName, time.Now().Format(time.RFC3339), skillName)

	scriptPath := filepath.Join(scriptsDir, "main.lua")
	return os.WriteFile(scriptPath, []byte(content), 0644)
}

func (c *Creator) CreateFromGap(gap, description string) error {
	skillName := sanitizeSkillName(gap)

	skill := SkillSpec{
		Name:        skillName,
		Description: description,
		Instructions: fmt.Sprintf(`Use this skill when: %s

## Execution
Execute the appropriate sub-scripts to handle this task.

## Error Handling
If execution fails, return error message with suggestion.`, gap),
		Runtime: "lua",
		Timeout: 30,
	}

	return c.Create(skill)
}

func (c *Creator) ListSkills() ([]string, error) {
	skillsDir := filepath.Join(c.workspace, "skills")

	entries, err := os.ReadDir(skillsDir)
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

func (c *Creator) Delete(skillName string) error {
	skillsDir := filepath.Join(c.workspace, "skills", skillName)
	return os.RemoveAll(skillsDir)
}

func sanitizeSkillName(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			if i > 0 && c >= 'A' && c <= 'Z' {
				result = append(result, '-')
			}
			result = append(result, c)
		}
	}
	if len(result) == 0 {
		result = append(result, "skill"...)
	}
	return string(result)
}
