package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type WebTool struct {
	httpCli *http.Client
}

func NewWebTool() *WebTool {
	return &WebTool{
		httpCli: &http.Client{},
	}
}

type DDGResult struct {
	Abstract      string            `json:"Abstract"`
	AbstractText  string            `json:"AbstractText"`
	Heading       string            `json:"Heading"`
	Answer        string            `json:"Answer"`
	Image         string            `json:"Image"`
	Results       []DDGSearchResult `json:"Results"`
	RelatedTopics []DDGRelatedTopic `json:"RelatedTopics"`
}

type DDGSearchResult struct {
	Title string `json:"Title"`
	URL   string `json:"URL"`
}

type DDGRelatedTopic struct {
	Name     string `json:"Name"`
	Text     string `json:"Text"`
	FirstURL string `json:"FirstURL"`
}

func (t *WebTool) Search(query string) (string, error) {
	baseURL := "https://api.duckduckgo.com/"

	params := url.Values{}
	params.Add("q", query)
	params.Add("format", "json")
	params.Add("pretty", "1")
	params.Add("no_html", "1")
	params.Add("skip_disambig", "1")
	params.Add("kl", "us-en")

	req, err := http.NewRequest("GET", baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Cluaw/1.0)")

	resp, err := t.httpCli.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result DDGResult
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	var output strings.Builder

	if result.Heading != "" {
		output.WriteString(fmt.Sprintf("# %s\n\n", result.Heading))
	}

	if result.AbstractText != "" {
		output.WriteString(result.AbstractText + "\n\n")
	}

	if result.Answer != "" {
		output.WriteString(fmt.Sprintf("Answer: %s\n\n", result.Answer))
	}

	if len(result.RelatedTopics) > 0 {
		output.WriteString("## Related Topics\n")
		for _, topic := range result.RelatedTopics[:5] {
			if topic.FirstURL != "" {
				output.WriteString(fmt.Sprintf("- [%s](%s)\n", topic.Name, topic.FirstURL))
			}
		}
	}

	if len(result.Results) > 0 {
		output.WriteString("\n## Search Results\n")
		for _, r := range result.Results[:5] {
			output.WriteString(fmt.Sprintf("- [%s](%s)\n", r.Title, r.URL))
		}
	}

	if output.Len() == 0 {
		return "No results found for: " + query, nil
	}

	return output.String(), nil
}

func (t *WebTool) Fetch(url string) (string, error) {
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Cluaw/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	content := string(body)

	maxLen := 8000
	if len(content) > maxLen {
		content = content[:maxLen] + "\n\n... [truncated]"
	}

	return content, nil
}
