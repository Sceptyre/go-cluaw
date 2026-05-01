package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sceptye/go-cluaw/internal/config"
	"github.com/sceptye/go-cluaw/internal/logger"
	"github.com/sceptye/go-cluaw/internal/lua"
)

// ScheduledTask represents a task that runs on a schedule
type ScheduledTask struct {
	Name         string    `json:"name"`
	Schedule     string    `json:"schedule"` // Cron expression
	Message      string    `json:"message"`  // Message to send to agent
	Enabled      bool      `json:"enabled"`
	MaxRunTimeMs int64     `json:"max_runtime_ms"`
	LastRun      time.Time `json:"last_run"`
	NextRun      time.Time `json:"next_run"`
	RunCount     int       `json:"run_count"`
}

// Config holds the scheduler configuration
type Config struct {
	Enabled     bool
	HeartbeatMs int64
	Timezone    string
	Tasks       []ScheduledTask
}

// Scheduler manages scheduled task execution
type Scheduler struct {
	cfg      *config.Config
	tasks    map[string]*ScheduledTask
	mu       sync.RWMutex
	running  bool
	log      *logger.Logger
	callback func(string) error // Called with message when task fires
	taskFile string
}

// New creates a new scheduler
func New(cfg *config.Config, log *logger.Logger, callback func(string) error) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		tasks:    make(map[string]*ScheduledTask),
		log:      log,
		callback: callback,
		taskFile: filepath.Join(cfg.ExpandWorkspace(), "scheduler", "tasks.json"),
	}
}

// Start begins the scheduler loop
func (s *Scheduler) Start() error {
	if !s.cfg.Scheduler.Enabled {
		s.log.Info("Scheduler: disabled in config")
		return nil
	}

	s.running = true

	// Load existing tasks from file
	if err := s.loadTasks(); err != nil {
		s.log.Warn("Scheduler: no existing tasks file, using config")
	}

	// Add tasks from config
	for _, t := range s.cfg.Scheduler.Tasks {
		if _, exists := s.tasks[t.Name]; !exists {
			s.tasks[t.Name] = &ScheduledTask{
				Name:         t.Name,
				Schedule:     t.Schedule,
				Message:      t.Message,
				Enabled:      t.Enabled,
				MaxRunTimeMs: t.MaxRunTimeMs,
			}
		}
	}

	// Calculate next run times
	s.calculateNextRuns()

	go s.runLoop()
	s.log.Info("Scheduler: started with %d tasks", len(s.tasks))
	return nil
}

// Stop halts the scheduler
func (s *Scheduler) Stop() {
	s.running = false
}

// AddTask creates a new scheduled task
func (s *Scheduler) AddTask(name, schedule, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate cron expression
	if _, err := ParseCron(schedule); err != nil {
		return fmt.Errorf("invalid cron: %w", err)
	}

	s.tasks[name] = &ScheduledTask{
		Name:     name,
		Schedule: schedule,
		Message:  message,
		Enabled:  true,
	}

	s.calculateNextRuns()
	s.saveTasks()
	s.log.Info("Scheduler: added task %s with schedule %s", name, schedule)
	return nil
}

// RemoveTask deletes a scheduled task
func (s *Scheduler) RemoveTask(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[name]; !exists {
		return fmt.Errorf("task not found: %s", name)
	}

	delete(s.tasks, name)
	s.saveTasks()
	s.log.Info("Scheduler: removed task %s", name)
	return nil
}

// ListTasks returns all scheduled tasks
func (s *Scheduler) ListTasks() []*lua.ScheduledTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*lua.ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, &lua.ScheduledTask{
			Name:         t.Name,
			Schedule:     t.Schedule,
			Message:      t.Message,
			Enabled:      t.Enabled,
			MaxRunTimeMs: t.MaxRunTimeMs,
			LastRun:      t.LastRun.Format("2006-01-02 15:04:05"),
			NextRun:      t.NextRun.Format("2006-01-02 15:04:05"),
			RunCount:     t.RunCount,
		})
	}
	return tasks
}

// runLoop is the main scheduler tick
func (s *Scheduler) runLoop() {
	ticker := time.NewTicker(time.Duration(s.cfg.Scheduler.HeartbeatMs) * time.Millisecond)
	defer ticker.Stop()

	for s.running {
		<-ticker.C
		s.tick()
	}
}

// tick checks if any tasks need to run
func (s *Scheduler) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for _, task := range s.tasks {
		if !task.Enabled {
			continue
		}

		if now.Sub(task.NextRun) >= 0 {
			s.log.Info("Scheduler: firing task %s", task.Name)

			// Fire the task via callback
			if s.callback != nil {
				go func(msg string) {
					if err := s.callback(msg); err != nil {
						s.log.Error("Scheduler: task %s error: %v", task.Name, err)
					}
				}(task.Message)
			}

			task.LastRun = now
			task.RunCount++
			s.calculateNextRun(task)
		}
	}

	s.saveTasks()
}

// calculateNextRuns updates next run times for all tasks
func (s *Scheduler) calculateNextRuns() {
	for _, task := range s.tasks {
		s.calculateNextRun(task)
	}
}

// calculateNextRun calculates the next run time for a task
func (s *Scheduler) calculateNextRun(task *ScheduledTask) {
	next, err := NextCronTime(task.Schedule, time.Now())
	if err != nil {
		task.NextRun = time.Now().Add(time.Hour) // Fallback
	} else {
		task.NextRun = next
	}
}

// loadTasks loads tasks from disk
func (s *Scheduler) loadTasks() error {
	data, err := os.ReadFile(s.taskFile)
	if err != nil {
		return err
	}

	var tasks []ScheduledTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return err
	}

	for i := range tasks {
		s.tasks[tasks[i].Name] = &tasks[i]
	}

	return nil
}

// saveTasks persists tasks to disk
func (s *Scheduler) saveTasks() error {
	dir := filepath.Dir(s.taskFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}

	data, err := json.Marshal(tasks)
	if err != nil {
		return err
	}

	return os.WriteFile(s.taskFile, data, 0644)
}
