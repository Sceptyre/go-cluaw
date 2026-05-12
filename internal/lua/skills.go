package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Version     string   `yaml:"version"`
	Author      string   `yaml:"author"`
	Runtime     string   `yaml:"runtime"`
	Timeout     int      `yaml:"timeout"`
	Permissions []string `yaml:"permissions"`
	Content     string
}

type SkillLoader struct {
	workspace string
}

func NewSkillLoader(workspace string) *SkillLoader {
	return &SkillLoader{workspace: workspace}
}

func (l *SkillLoader) List() ([]string, error) {
	entries, err := os.ReadDir(l.workspace + "/skills")
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

func (l *SkillLoader) Load(name string) (*Skill, error) {
	path := filepath.Join(l.workspace, "skills", name, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill: %w", err)
	}

	skill, err := parseSkillFile(data)
	if err != nil {
		return nil, fmt.Errorf("parse skill: %w", err)
	}

	return skill, nil
}

func (l *SkillLoader) Save(skill *Skill) error {
	dir := filepath.Join(l.workspace, "skills", skill.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	content := formatSkillFile(skill)
	path := filepath.Join(dir, "SKILL.md")

	return os.WriteFile(path, []byte(content), 0644)
}

func parseSkillFile(data []byte) (*Skill, error) {
	parts := strings.Split(string(data), "---")
	if len(parts) < 3 {
		return &Skill{Content: string(data)}, nil
	}

	yamlContent := strings.TrimSpace(parts[1])
	markdown := strings.TrimSpace(strings.Join(parts[2:], "---"))

	var skill Skill
	if err := yaml.Unmarshal([]byte(yamlContent), &skill); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	skill.Content = markdown
	return &skill, nil
}

func formatSkillFile(skill *Skill) string {
	metadata := fmt.Sprintf(`---
name: %s
description: %s
version: %s
author: %s
runtime: %s
timeout: %d
---

%s`, skill.Name, skill.Description, skill.Version, skill.Author, skill.Runtime, skill.Timeout, skill.Content)
	return metadata
}

func (l *SkillLoader) GetScriptPath(skillName, scriptName string) string {
	return filepath.Join(l.workspace, "skills", skillName, "scripts", scriptName+".lua")
}

func (l *SkillLoader) Workspace() string {
	return l.workspace
}

func (l *SkillLoader) ListScripts(skillName string) ([]string, error) {
	path := filepath.Join(l.workspace, "skills", skillName, "scripts")
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var scripts []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".lua") {
			scripts = append(scripts, strings.TrimSuffix(e.Name(), ".lua"))
		}
	}
	return scripts, nil
}
