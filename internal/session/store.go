package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	dir string
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type ToolCall struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Output string          `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type Session struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	CreatedAt time.Time  `json:"created_at"`
	Messages  []Message  `json:"messages"`
	ToolCalls []ToolCall `json:"tool_calls"`
	Active    bool       `json:"active"`
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Create(userID string) *Session {
	return &Session{
		ID:        generateSessionID(),
		UserID:    userID,
		CreatedAt: time.Now(),
		Messages:  make([]Message, 0),
		ToolCalls: make([]ToolCall, 0),
		Active:    true,
	}
}

func (s *Store) Save(sess *Session) error {
	sess.Active = false

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	filename := filepath.Join(s.dir, sess.ID+".jsonl")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	_, err = f.Write([]byte("\n"))
	return err
}

func (s *Store) Load(id string) (*Session, error) {
	filename := filepath.Join(s.dir, id+".jsonl")
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &sess, nil
}

// LoadByUser finds the most recent session for a given userID.
// It scans session files and returns the latest one matching the user.
func (s *Store) LoadByUser(userID string) (*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var latest *Session
	var latestTime time.Time

	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}

		name := e.Name()
		filename := filepath.Join(s.dir, name)
		data, err := os.ReadFile(filename)
		if err != nil {
			continue
		}

		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}

		if sess.UserID == userID && sess.CreatedAt.After(latestTime) {
			latestTime = sess.CreatedAt
			latest = &sess
		}
	}

	if latest == nil {
		return nil, fmt.Errorf("no session found for user: %s", userID)
	}

	return latest, nil
}

func (s *Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var sessions []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".jsonl" {
			sessions = append(sessions, e.Name())
		}
	}
	return sessions, nil
}

func generateSessionID() string {
	now := time.Now()
	return fmt.Sprintf("sess-%s-%d", now.Format("20060102-150405"), now.UnixNano()%10000)
}
