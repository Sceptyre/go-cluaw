package memory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Index struct {
	workspace string
}

func NewIndex(workspace string) *Index {
	return &Index{workspace: workspace}
}

func (i *Index) Search(query string) (string, error) {
	memoryDir := filepath.Join(i.workspace, "memory")

	if _, err := os.Stat(memoryDir); os.IsNotExist(err) {
		return "No memories found", nil
	}

	cmd := exec.Command("grep", "-r", "-i", "--include=*.md", "-l", query, memoryDir)
	output, err := cmd.Output()
	if err != nil {
		if strings.Contains(err.Error(), "exit status 1") {
			return "No matches found for: " + query, nil
		}
		return "", err
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var results []string

	for _, file := range files {
		if file == "" {
			continue
		}
		relPath, _ := filepath.Rel(memoryDir, file)
		results = append(results, relPath)
	}

	if len(results) == 0 {
		return "No matches found for: " + query, nil
	}

	outputStr := fmt.Sprintf("Found in %d files:\n", len(results))
	for _, f := range results {
		outputStr += fmt.Sprintf("- %s\n", f)
	}

	return outputStr, nil
}

func (i *Index) SearchByTag(tag string) (string, error) {
	return i.Search("#" + tag)
}

func (i *Index) AddEntry(file, tag, entry string) error {
	memoryDir := filepath.Join(i.workspace, "memory")
	indexPath := filepath.Join(memoryDir, "index.md")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	newLine := fmt.Sprintf("- %s: [[%s]]", file, tag)

	content := string(data)
	if !strings.Contains(content, newLine) {
		if tag != "" {
			tagSection := "## By Tag"
			if idx := strings.Index(content, tagSection); idx != -1 {
				insertIdx := idx + len(tagSection) + 1
				atIdx := strings.Index(content[insertIdx:], "##")
				if atIdx == -1 {
					content = content + "\n" + newLine + "\n"
				} else {
					content = content[:insertIdx+atIdx] + newLine + "\n" + content[insertIdx+atIdx:]
				}
			}
		}
	}

	return os.WriteFile(indexPath, []byte(content), 0644)
}

func (i *Index) ListFiles() ([]string, error) {
	memoryDir := filepath.Join(i.workspace, "memory")

	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			files = append(files, e.Name())
		}
	}

	return files, nil
}

func (i *Index) GetRecentFiles(count int) ([]string, error) {
	files, err := i.ListFiles()
	if err != nil {
		return nil, err
	}

	if count > 0 && len(files) > count {
		files = files[:count]
	}

	return files, nil
}

func (i *Index) FindByDate(date string) (string, error) {
	memoryDir := filepath.Join(i.workspace, "memory")
	path := filepath.Join(memoryDir, date+".md")

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (i *Index) GetReference(ref string) (string, error) {
	re := regexp.MustCompile(`\[\[([^]]+)\]\]`)
	matches := re.FindStringSubmatch(ref)
	if len(matches) < 2 {
		return "", fmt.Errorf("invalid reference format")
	}

	refFile := matches[1]
	path := filepath.Join(i.workspace, "memory", refFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
