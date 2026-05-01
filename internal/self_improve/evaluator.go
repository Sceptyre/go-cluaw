package self_improve

import (
	"fmt"
	"os"
	"path/filepath"
)

type Evaluator struct {
	workspace string
	scores    map[string]SkillScore
}

type SkillScore struct {
	Name            string  `json:"name"`
	Correctness     int     `json:"correctness"`     // 0-10
	Security        int     `json:"security"`        // 0-10
	Completeness    int     `json:"completeness"`    // 0-10
	Performance     int     `json:"performance"`     // 0-10
	Maintainability int     `json:"maintainability"` // 0-10
	Documentation   int     `json:"documentation"`   // 0-10
	Total           float64 `json:"total"`
}

func NewEvaluator(workspace string) *Evaluator {
	return &Evaluator{
		workspace: workspace,
		scores:    make(map[string]SkillScore),
	}
}

func (e *Evaluator) Evaluate(skillName string) (SkillScore, error) {
	path := filepath.Join(e.workspace, "skills", skillName, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillScore{}, err
	}

	score := e.evaluateContent(string(data))
	score.Name = skillName
	score.Total = e.calculateTotal(score)

	e.scores[skillName] = score
	return score, nil
}

func (e *Evaluator) evaluateContent(content string) SkillScore {
	score := SkillScore{}

	score.Correctness = e.rateCorrectness(content)
	score.Security = e.rateSecurity(content)
	score.Completeness = e.rateCompleteness(content)
	score.Performance = e.ratePerformance(content)
	score.Maintainability = e.rateMaintainability(content)
	score.Documentation = e.rateDocumentation(content)

	return score
}

func (e *Evaluator) rateCorrectness(content string) int {
	score := 7

	if hasYAMLFrontmatter(content) {
		score += 1
	}
	if hasNameField(content) {
		score += 1
	}
	if hasDescriptionField(content) {
		score += 1
	}

	return minInt(score, 10)
}

func (e *Evaluator) rateSecurity(content string) int {
	score := 6

	if !containsStr(content, "os.execute") {
		score += 1
	}
	if !containsStr(content, "os.exit") {
		score += 1
	}
	if containsStr(content, "sandbox") || containsStr(content, "secure") {
		score += 1
	}

	return minInt(score, 10)
}

func (e *Evaluator) rateCompleteness(content string) int {
	score := 5

	if containsStr(content, "# ") {
		score += 1
	}
	if containsStr(content, "##") {
		score += 1
	}
	if countHeaders(content) >= 3 {
		score += 2
	}

	return minInt(score, 10)
}

func (e *Evaluator) ratePerformance(content string) int {
	score := 7

	if containsStr(content, "timeout") {
		score += 1
	}
	if containsStr(content, "limit") {
		score += 1
	}

	return minInt(score, 10)
}

func (e *Evaluator) rateMaintainability(content string) int {
	score := 5

	if containsStr(content, "function") {
		score += 2
	}
	if containsStr(content, "local") {
		score += 1
	}

	return minInt(score, 10)
}

func (e *Evaluator) rateDocumentation(content string) int {
	score := 6

	if containsStr(content, "Usage") || containsStr(content, "usage") {
		score += 1
	}
	if countHeaders(content) >= 2 {
		score += 2
	}

	return minInt(score, 10)
}

func (e *Evaluator) calculateTotal(s SkillScore) float64 {
	total := float64(s.Correctness+s.Security+s.Completeness+s.Performance+s.Maintainability+s.Documentation) / 6
	return total
}

func (e *Evaluator) ShouldRollback(skillName string) bool {
	score, ok := e.scores[skillName]
	if !ok {
		score, err := e.Evaluate(skillName)
		if err != nil {
			return true
		}
		e.scores[skillName] = score
	}

	dropTolerance := 2
	previous, ok := e.scores[skillName+"_previous"]
	if ok {
		previousScore := e.calculateTotal(previous)
		currentScore := e.calculateTotal(score)
		return (previousScore - currentScore) >= float64(dropTolerance)
	}

	return score.Total < 5.0
}

func (e *Evaluator) GetScores() map[string]SkillScore {
	return e.scores
}

func (e *Evaluator) savePreviousScore(skillName string) {
	if score, ok := e.scores[skillName]; ok {
		e.scores[skillName+"_previous"] = score
	}
}

func hasYAMLFrontmatter(s string) bool {
	return len(s) >= 3 && s[:3] == "---"
}

func hasNameField(s string) bool {
	return containsStr(s, "name:") || containsStr(s, "name:")
}

func hasDescriptionField(s string) bool {
	return containsStr(s, "description:") || containsStr(s, "description:")
}

func countHeaders(s string) int {
	count := 0
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '\n' && s[i+1] == '#' {
			count++
		}
	}
	return count
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = fmt.Sprintf
var _ = os.ReadFile
var _ = filepath.Join
