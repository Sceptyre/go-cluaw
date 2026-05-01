package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// ScheduledTask represents a scheduled task (mirrored from scheduler package)
type ScheduledTask struct {
	Name         string `json:"name"`
	Schedule     string `json:"schedule"`
	Message      string `json:"message"`
	Enabled      bool   `json:"enabled"`
	MaxRunTimeMs int64  `json:"max_runtime_ms"`
	LastRun      string `json:"last_run"`
	NextRun      string `json:"next_run"`
	RunCount     int    `json:"run_count"`
}

type Sandbox struct {
	timeout      time.Duration
	allowedPaths []string
	scheduler    interface { // Circular dependency: scheduler interface
		AddTask(name, schedule, message string) error
		RemoveTask(name string) error
		ListTasks() []*ScheduledTask
	}
}

func New(timeout time.Duration, allowedPaths []string, scheduler interface {
	AddTask(name, schedule, message string) error
	RemoveTask(name string) error
	ListTasks() []*ScheduledTask
}) *Sandbox {
	return &Sandbox{
		timeout:      timeout,
		allowedPaths: allowedPaths,
		scheduler:    scheduler,
	}
}

// SetScheduler sets the scheduler (can be called after New if scheduler wasn't available)
func (s *Sandbox) SetScheduler(scheduler interface {
	AddTask(name, schedule, message string) error
	RemoveTask(name string) error
	ListTasks() []*ScheduledTask
}) {
	s.scheduler = scheduler
}

func (s *Sandbox) Execute(script string, args string) (string, error) {
	L := lua.NewState()
	defer L.Close()

	// Local buffer to capture print output (thread-safe, per-call)
	var printBuf strings.Builder
	var mu sync.Mutex // Protects buffer if handler could be called concurrently

	s.setupEnvironment(L, &printBuf, &mu)

	// Load and execute the script
	if err := L.DoString(script); err != nil {
		return "", fmt.Errorf("lua error: %w", err)
	}

	// Get the result from the script (it might be a function or value)
	top := L.GetTop()
	if top > 0 {
		result := L.Get(-1)

		// If result is a function, call it
		if fn, ok := result.(*lua.LFunction); ok && fn != nil {
			// Push args as table if provided
			if args != "" {
				tbl := L.CreateTable(1, 0)
				tbl.RawSetInt(1, lua.LString(args))
				L.Push(tbl)
			} else {
				L.Push(lua.LNil)
			}
			// Call the function with 1 argument and expect 1 return
			L.Call(1, 1)
		}
	}

	retVal := s.getReturnValue(L)
	printOut := printBuf.String()

	// Combine print output with return value
	if printOut != "" && retVal != "" {
		return printOut + "\n" + retVal, nil
	} else if printOut != "" {
		return printOut, nil
	}
	return retVal, nil
}

func (s *Sandbox) setupEnvironment(L *lua.LState, printBuf *strings.Builder, mu *sync.Mutex) {
	L.SetGlobal("io", lua.LNil)

	// Create os table with safe functions
	osTable := L.NewTable()
	// Block dangerous functions
	L.SetField(osTable, "execute", lua.LNil)
	L.SetField(osTable, "remove", lua.LNil)
	L.SetField(osTable, "rename", lua.LNil)
	L.SetField(osTable, "getenv", lua.LNil)
	L.SetField(osTable, "exit", lua.LNil)
	// Keep safe functions
	L.SetField(osTable, "date", L.NewFunction(s.OsDateHandler))
	L.SetField(osTable, "time", L.NewFunction(s.OsTimeHandler))
	L.SetGlobal("os", osTable)
	L.SetGlobal("load", lua.LNil)
	L.SetGlobal("loadfile", lua.LNil)
	L.SetGlobal("loadstring", lua.LNil)
	L.SetGlobal("dofile", lua.LNil)
	L.SetGlobal("debug", lua.LNil)

	L.SetGlobal("print", L.NewFunction(s.makePrintHandler(printBuf, mu)))
	L.SetGlobal("file", L.NewFunction(s.fileHandler))
	L.SetGlobal("http", L.NewFunction(s.httpHandler))
	L.SetGlobal("help", L.NewFunction(s.helpHandler))

	// Create time module table
	timeTable := L.NewTable()
	L.SetField(timeTable, "time", L.NewFunction(s.OsTimeHandler))
	L.SetField(timeTable, "date", L.NewFunction(s.OsDateHandler))
	L.SetGlobal("time", timeTable)

	// Create scheduler module table
	schedulerTable := L.NewTable()
	L.SetField(schedulerTable, "add", L.NewFunction(s.schedulerAddHandler))
	L.SetField(schedulerTable, "remove", L.NewFunction(s.schedulerRemoveHandler))
	L.SetField(schedulerTable, "list", L.NewFunction(s.schedulerListHandler))
	L.SetGlobal("scheduler", schedulerTable)
}

// makePrintHandler creates a print handler closure that captures output to the provided buffer
func (s *Sandbox) makePrintHandler(printBuf *strings.Builder, mu *sync.Mutex) func(*lua.LState) int {
	return func(L *lua.LState) int {
		n := L.GetTop()
		var args []string
		for i := 1; i <= n; i++ {
			args = append(args, fmt.Sprintf("%v", L.Get(i)))
		}
		output := strings.Join(args, " ")
		mu.Lock()
		defer mu.Unlock()
		if printBuf.Len() > 0 {
			printBuf.WriteString("\n")
		}
		printBuf.WriteString(output)
		return 0
	}
}

func (s *Sandbox) fileHandler(L *lua.LState) int {
	op := L.Get(1).String()
	path := L.Get(2).String()

	if !s.isPathAllowed(path) {
		L.Push(lua.LString("Error: path not allowed: " + path))
		return 1
	}

	switch op {
	case "read":
		data, err := os.ReadFile(path)
		if err != nil {
			L.Push(lua.LString(fmt.Sprintf("Error: %v", err)))
			return 1
		}
		L.Push(lua.LString(string(data)))
	case "write":
		content := L.Get(3).String()
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			L.Push(lua.LString(fmt.Sprintf("Error: %v", err)))
			return 1
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			L.Push(lua.LString(fmt.Sprintf("Error: %v", err)))
			return 1
		}
		L.Push(lua.LString("OK"))
	case "list":
		entries, err := os.ReadDir(path)
		if err != nil {
			L.Push(lua.LString(fmt.Sprintf("Error: %v", err)))
			return 1
		}
		var files []string
		for _, e := range entries {
			files = append(files, e.Name())
		}
		L.Push(lua.LString(strings.Join(files, "\n")))
	default:
		L.Push(lua.LString("Error: unknown operation: " + op))
		return 1
	}

	return 1
}

func (s *Sandbox) httpHandler(L *lua.LState) int {
	method := L.Get(1).String()
	url := L.Get(2).String()

	L.Push(lua.LString(fmt.Sprintf("HTTP %s %s - use Go HTTP client in cluaw", method, url)))
	return 1
}

func (s *Sandbox) OsTimeHandler(L *lua.LState) int {
	now := time.Now().Unix()
	L.Push(lua.LNumber(now))
	return 1
}

func (s *Sandbox) OsDateHandler(L *lua.LState) int {
	now := time.Now().Format("2006-01-02 15:04:05")
	L.Push(lua.LString(now))
	return 1
}

func (s *Sandbox) schedulerAddHandler(L *lua.LState) int {
	if s.scheduler == nil {
		L.Push(lua.LString("Error: scheduler not available"))
		return 1
	}

	name := L.Get(1).String()
	schedule := L.Get(2).String()
	message := L.Get(3).String()

	if err := s.scheduler.AddTask(name, schedule, message); err != nil {
		L.Push(lua.LString(fmt.Sprintf("Error: %v", err)))
		return 1
	}

	L.Push(lua.LString("OK: task " + name + " scheduled"))
	return 1
}

func (s *Sandbox) schedulerRemoveHandler(L *lua.LState) int {
	if s.scheduler == nil {
		L.Push(lua.LString("Error: scheduler not available"))
		return 1
	}

	name := L.Get(1).String()

	if err := s.scheduler.RemoveTask(name); err != nil {
		L.Push(lua.LString(fmt.Sprintf("Error: %v", err)))
		return 1
	}

	L.Push(lua.LString("OK: task " + name + " removed"))
	return 1
}

func (s *Sandbox) schedulerListHandler(L *lua.LState) int {
	if s.scheduler == nil {
		L.Push(lua.LString("Error: scheduler not available"))
		return 1
	}

	tasks := s.scheduler.ListTasks()
	var result []string
	for _, t := range tasks {
		result = append(result, fmt.Sprintf("%s: %s -> %s", t.Name, t.Schedule, t.Message))
	}

	if len(result) == 0 {
		L.Push(lua.LString("No scheduled tasks"))
	} else {
		L.Push(lua.LString(strings.Join(result, "\n")))
	}
	return 1
}

func (s *Sandbox) helpHandler(L *lua.LState) int {
	n := L.GetTop()
	if n == 0 {
		// Return list of all available modules
		L.Push(lua.LString(`Available modules:
- scheduler: Manage scheduled tasks (scheduler.add, scheduler.remove, scheduler.list)
- file: File operations (file.read, file.write, file.list)
- http: HTTP requests (http.get, http.post)
- os: Date/time (os.date, os.time)
- time: Time utilities (time.time, time.date)

Use help("module") or help("module.function") for more info.`))
		return 1
	}

	module := L.Get(1).String()

	switch module {
	case "scheduler":
		L.Push(lua.LString(`scheduler - Manage scheduled tasks

Functions:
- scheduler.add(name, schedule, message): Add a new scheduled task
  Example: scheduler.add("my_task", "0 * * * *", "Hello world")
- scheduler.remove(name): Remove a scheduled task
  Example: scheduler.remove("my_task")
- scheduler.list(): List all scheduled tasks`))
	case "scheduler.add":
		L.Push(lua.LString(`scheduler.add(name, schedule, message)

Add a new scheduled task.

Parameters:
- name: Unique name for the task
- schedule: Cron expression (e.g., "0 * * * *" for every hour)
- message: Message to display when task runs

Example: scheduler.add("daily_report", "0 9 * * *", "Generate report")`))
	case "scheduler.remove":
		L.Push(lua.LString(`scheduler.remove(name)

Remove a scheduled task by name.

Parameter:
- name: The name of the task to remove

Example: scheduler.remove("daily_report")`))
	case "scheduler.list":
		L.Push(lua.LString(`scheduler.list()

List all scheduled tasks with their schedules and messages.

Returns: String with task information or "No scheduled tasks"`))
	case "file":
		L.Push(lua.LString(`file - File operations

Functions:
- file.read(path): Read file contents
  Example: file.read("/path/to/file.txt")
- file.write(path, content): Write content to file
  Example: file.write("/path/to/file.txt", "Hello world")
- file.list(path): List directory contents
  Example: file.list("/path/to/directory")`))
	case "file.read":
		L.Push(lua.LString(`file.read(path)

Read the contents of a file.

Parameter:
- path: Absolute path to the file

Returns: File contents as string or error message`))
	case "file.write":
		L.Push(lua.LString(`file.write(path, content)

Write content to a file. Creates parent directories if needed.

Parameters:
- path: Absolute path to the file
- content: String content to write

Returns: "OK" on success or error message`))
	case "file.list":
		L.Push(lua.LString(`file.list(path)

List directory contents.

Parameter:
- path: Absolute path to directory

Returns: List of filenames separated by newlines or error message`))
	case "http":
		L.Push(lua.LString(`http - HTTP requests

Functions:
- http.get(url): Make GET request
- http.post(url, body): Make POST request

Note: Currently provides placeholder functionality.`))
	case "http.get":
		L.Push(lua.LString(`http.get(url)

Make an HTTP GET request.

Parameter:
- url: The URL to request

Returns: Placeholder response string`))
	case "http.post":
		L.Push(lua.LString(`http.post(url, body)

Make an HTTP POST request.

Parameters:
- url: The URL to request
- body: Request body content

Returns: Placeholder response string`))
	case "os":
		L.Push(lua.LString(`os - Operating system utilities

Functions:
- os.date(): Get current date/time
  Returns: Current date in "YYYY-MM-DD HH:MM:SS" format
- os.time(): Get current Unix timestamp
  Returns: Current Unix timestamp as number`))
	case "os.date":
		L.Push(lua.LString(`os.date()

Get current date and time.

Returns: Current date/time in "YYYY-MM-DD HH:MM:SS" format

Example: print(os.date())`))
	case "os.time":
		L.Push(lua.LString(`os.time()

Get current Unix timestamp.

Returns: Current Unix timestamp as a number

Example: print(os.time())`))
	case "time":
		L.Push(lua.LString(`time - Time utilities

Functions:
- time.time(): Get current Unix timestamp
- time.date(): Get current date/time

Note: Aliases for os.time and os.date.`))
	case "time.time":
		L.Push(lua.LString(`time.time()

Get current Unix timestamp.

Returns: Current Unix timestamp as a number

Example: print(time.time())`))
	case "time.date":
		L.Push(lua.LString(`time.date()

Get current date and time.

Returns: Current date/time in "YYYY-MM-DD HH:MM:SS" format

Example: print(time.date())`))
	default:
		L.Push(lua.LString("Unknown module or function: " + module + "\nUse help() to see available modules."))
	}
	return 1
}

func (s *Sandbox) isPathAllowed(path string) bool {
	if path == "" {
		return false
	}
	for _, allowed := range s.allowedPaths {
		if strings.HasPrefix(path, allowed) {
			return true
		}
	}
	return false
}

func (s *Sandbox) getReturnValue(L *lua.LState) string {
	if L.GetTop() > 0 {
		return fmt.Sprintf("%v", L.Get(-1))
	}
	return ""
}
