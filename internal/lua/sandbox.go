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

// Interfaces for tool dependencies (avoids circular imports with tools/ package)

type FileOperator interface {
	Read(path string) (string, error)
	Write(path, content string) (string, error)
	Edit(path, search, replace string) (string, error)
	List(dir string) ([]string, error)
}

type WebOperator interface {
	Fetch(url string) (string, error)
	Search(query string) (string, error)
}

type BrowserOperator interface {
	Navigate(url string) (string, error)
	Click(selector string) (string, error)
	Type(selector, text string) (string, error)
	Screenshot() ([]byte, error)
}

type MemoryOperator interface {
	Today() (string, error)
	ByReference(ref string) (string, error)
	ByTag(tag string) (string, error)
	ByDate(date string) (string, error)
	AppendToToday(entry string) error
	Search(query string) (string, error)
}

type ScheduleOperator interface {
	AddTask(name, schedule, message string) error
	RemoveTask(name string) error
	ListTasks() []*ScheduledTask
}

type Sandbox struct {
	timeout      time.Duration
	allowedPaths []string
	scheduler    ScheduleOperator
	fileOp       FileOperator
	webOp        WebOperator
	browserOp    BrowserOperator
	memoryOp     MemoryOperator
	skills       *SkillLoader
}

func New(timeout time.Duration, allowedPaths []string, sched ScheduleOperator) *Sandbox {
	return &Sandbox{
		timeout:      timeout,
		allowedPaths: allowedPaths,
		scheduler:    sched,
	}
}

func NewWithTools(timeout time.Duration, allowedPaths []string, scheduler ScheduleOperator,
	fileOp FileOperator, webOp WebOperator, browserOp BrowserOperator,
	memoryOp MemoryOperator, skills *SkillLoader) *Sandbox {
	return &Sandbox{
		timeout:      timeout,
		allowedPaths: allowedPaths,
		scheduler:    scheduler,
		fileOp:       fileOp,
		webOp:        webOp,
		browserOp:    browserOp,
		memoryOp:     memoryOp,
		skills:       skills,
	}
}

// SetScheduler sets the scheduler (can be called after New if scheduler wasn't available)
func (s *Sandbox) SetScheduler(sched ScheduleOperator) {
	s.scheduler = sched
}

// SetToolDependencies sets all tool dependencies (for late-binding)
func (s *Sandbox) SetToolDependencies(fileOp FileOperator, webOp WebOperator, browserOp BrowserOperator, memoryOp MemoryOperator, skills *SkillLoader) {
	s.fileOp = fileOp
	s.webOp = webOp
	s.browserOp = browserOp
	s.memoryOp = memoryOp
	s.skills = skills
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
	L.SetGlobal("load", lua.LNil)
	L.SetGlobal("loadfile", lua.LNil)
	L.SetGlobal("loadstring", lua.LNil)
	L.SetGlobal("dofile", lua.LNil)
	L.SetGlobal("debug", lua.LNil)

	// os module — safe functions only
	osTable := L.NewTable()
	L.SetField(osTable, "execute", lua.LNil)
	L.SetField(osTable, "remove", lua.LNil)
	L.SetField(osTable, "rename", lua.LNil)
	L.SetField(osTable, "getenv", lua.LNil)
	L.SetField(osTable, "exit", lua.LNil)
	L.SetField(osTable, "date", L.NewFunction(s.OsDateHandler))
	L.SetField(osTable, "time", L.NewFunction(s.OsTimeHandler))
	L.SetGlobal("os", osTable)

	// print — captured to result buffer
	L.SetGlobal("print", L.NewFunction(s.makePrintHandler(printBuf, mu)))

	// help — inline documentation
	L.SetGlobal("help", L.NewFunction(s.helpHandler))

	// time module
	timeTable := L.NewTable()
	L.SetField(timeTable, "time", L.NewFunction(s.OsTimeHandler))
	L.SetField(timeTable, "date", L.NewFunction(s.OsDateHandler))
	L.SetGlobal("time", timeTable)

	// file module — read / write / edit / list
	fileTable := L.NewTable()
	L.SetField(fileTable, "read", L.NewFunction(s.fileReadHandler))
	L.SetField(fileTable, "write", L.NewFunction(s.fileWriteHandler))
	L.SetField(fileTable, "edit", L.NewFunction(s.fileEditHandler))
	L.SetField(fileTable, "list", L.NewFunction(s.fileListHandler))
	L.SetGlobal("file", fileTable)

	// web module — fetch / search
	webTable := L.NewTable()
	L.SetField(webTable, "fetch", L.NewFunction(s.webFetchHandler))
	L.SetField(webTable, "search", L.NewFunction(s.webSearchHandler))
	L.SetGlobal("web", webTable)

	// browser module — navigate / click / type / screenshot
	browserTable := L.NewTable()
	L.SetField(browserTable, "navigate", L.NewFunction(s.browserNavigateHandler))
	L.SetField(browserTable, "click", L.NewFunction(s.browserClickHandler))
	L.SetField(browserTable, "type", L.NewFunction(s.browserTypeHandler))
	L.SetField(browserTable, "screenshot", L.NewFunction(s.browserScreenshotHandler))
	L.SetGlobal("browser", browserTable)

	// memory module — search / get / write / today
	memoryTable := L.NewTable()
	L.SetField(memoryTable, "search", L.NewFunction(s.memorySearchHandler))
	L.SetField(memoryTable, "get", L.NewFunction(s.memoryGetHandler))
	L.SetField(memoryTable, "write", L.NewFunction(s.memoryWriteHandler))
	L.SetField(memoryTable, "today", L.NewFunction(s.memoryTodayHandler))
	L.SetGlobal("memory", memoryTable)

	// skill module — list / exec / create
	skillTable := L.NewTable()
	L.SetField(skillTable, "list", L.NewFunction(s.skillListHandler))
	L.SetField(skillTable, "exec", L.NewFunction(s.skillExecHandler))
	L.SetField(skillTable, "create", L.NewFunction(s.skillCreateHandler))
	L.SetGlobal("skill", skillTable)

	// scheduler module — add / remove / list
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

func (s *Sandbox) fileReadHandler(L *lua.LState) int {
	if !s.hasFileOp(L) {
		return 1
	}
	path := L.CheckString(1)
	if !s.isPathAllowed(path) {
		L.Push(lua.LString("Error: path not allowed: " + path))
		return 1
	}
	result, err := s.fileOp.Read(path)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) fileWriteHandler(L *lua.LState) int {
	if !s.hasFileOp(L) {
		return 1
	}
	path := L.CheckString(1)
	content := L.CheckString(2)
	if !s.isPathAllowed(path) {
		L.Push(lua.LString("Error: path not allowed: " + path))
		return 1
	}
	result, err := s.fileOp.Write(path, content)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) fileEditHandler(L *lua.LState) int {
	if !s.hasFileOp(L) {
		return 1
	}
	path := L.CheckString(1)
	search := L.CheckString(2)
	replace := L.CheckString(3)
	if !s.isPathAllowed(path) {
		L.Push(lua.LString("Error: path not allowed: " + path))
		return 1
	}
	result, err := s.fileOp.Edit(path, search, replace)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) fileListHandler(L *lua.LState) int {
	if !s.hasFileOp(L) {
		return 1
	}
	path := L.CheckString(1)
	if !s.isPathAllowed(path) {
		L.Push(lua.LString("Error: path not allowed: " + path))
		return 1
	}
	entries, err := s.fileOp.List(path)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	if len(entries) == 0 {
		L.Push(lua.LString("(empty)"))
	} else {
		L.Push(lua.LString(strings.Join(entries, "\n")))
	}
	return 1
}

func (s *Sandbox) webFetchHandler(L *lua.LState) int {
	if !s.hasWebOp(L) {
		return 1
	}
	url := L.CheckString(1)
	result, err := s.webOp.Fetch(url)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) webSearchHandler(L *lua.LState) int {
	if !s.hasWebOp(L) {
		return 1
	}
	query := L.CheckString(1)
	result, err := s.webOp.Search(query)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) browserNavigateHandler(L *lua.LState) int {
	if !s.hasBrowserOp(L) {
		return 1
	}
	url := L.CheckString(1)
	result, err := s.browserOp.Navigate(url)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) browserClickHandler(L *lua.LState) int {
	if !s.hasBrowserOp(L) {
		return 1
	}
	selector := L.CheckString(1)
	result, err := s.browserOp.Click(selector)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) browserTypeHandler(L *lua.LState) int {
	if !s.hasBrowserOp(L) {
		return 1
	}
	selector := L.CheckString(1)
	text := L.CheckString(2)
	result, err := s.browserOp.Type(selector, text)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) browserScreenshotHandler(L *lua.LState) int {
	if !s.hasBrowserOp(L) {
		return 1
	}
	data, err := s.browserOp.Screenshot()
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(fmt.Sprintf("[screenshot: %d bytes]", len(data))))
	return 1
}

func (s *Sandbox) memorySearchHandler(L *lua.LState) int {
	if !s.hasMemoryOp(L) {
		return 1
	}
	query := L.CheckString(1)
	result, err := s.memoryOp.Search(query)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) memoryGetHandler(L *lua.LState) int {
	if !s.hasMemoryOp(L) {
		return 1
	}
	n := L.GetTop()
	var result string
	var err error

	if n >= 1 {
		arg := L.Get(1)
		switch v := arg.(type) {
		case lua.LString:
			// Check if it's a date pattern YYYY-MM-DD
			if len(v) == 10 && v[4] == '-' && v[7] == '-' {
				result, err = s.memoryOp.ByDate(string(v))
			} else {
				result, err = s.memoryOp.ByReference(string(v))
			}
		default:
			result, err = s.memoryOp.Today()
		}
	} else {
		result, err = s.memoryOp.Today()
	}

	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}

func (s *Sandbox) memoryWriteHandler(L *lua.LState) int {
	if !s.hasMemoryOp(L) {
		return 1
	}
	entry := L.CheckString(1)
	if err := s.memoryOp.AppendToToday(entry); err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString("OK: memory updated"))
	return 1
}

func (s *Sandbox) memoryTodayHandler(L *lua.LState) int {
	if !s.hasMemoryOp(L) {
		return 1
	}
	result, err := s.memoryOp.Today()
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(lua.LString(result))
	return 1
}


func (s *Sandbox) skillListHandler(L *lua.LState) int {
	if s.skills == nil {
		L.Push(lua.LString("Error: skills not available"))
		return 1
	}
	skills, err := s.skills.List()
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	if len(skills) == 0 {
		L.Push(lua.LString("No skills available"))
		return 1
	}
	L.Push(lua.LString(strings.Join(skills, "\n")))
	return 1
}

func (s *Sandbox) skillExecHandler(L *lua.LState) int {
	if s.skills == nil {
		L.Push(lua.LString("Error: skills not available"))
		return 1
	}
	skillName := L.CheckString(1)
	scriptName := ""
	args := ""
	if L.GetTop() >= 2 {
		scriptName = L.CheckString(2)
	}
	if L.GetTop() >= 3 {
		args = L.CheckString(3)
	}

	var scriptPath string
	if scriptName != "" {
		scriptPath = s.skills.GetScriptPath(skillName, scriptName)
	} else {
		scripts, err := s.skills.ListScripts(skillName)
		if err != nil || len(scripts) == 0 {
			L.Push(lua.LString("Error: no scripts for skill '" + skillName + "'"))
			return 1
		}
		scriptPath = s.skills.GetScriptPath(skillName, scripts[0])
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}

	fn, err := L.LoadString(string(data))
	if err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	L.Push(fn)
	if args != "" {
		L.Push(lua.LString(args))
	} else {
		L.Push(lua.LNil)
	}
	if err := L.PCall(1, 1, nil); err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	return 1
}

func (s *Sandbox) skillCreateHandler(L *lua.LState) int {
	if s.skills == nil {
		L.Push(lua.LString("Error: skills not available"))
		return 1
	}
	name := L.CheckString(1)
	description := ""
	instructions := ""
	if L.GetTop() >= 2 {
		description = L.CheckString(2)
	}
	if L.GetTop() >= 3 {
		instructions = L.CheckString(3)
	}

	sk := &Skill{
		Name:        name,
		Description: description,
		Version:     "1.0.0",
		Author:      "cluaw",
		Runtime:     "lua",
		Timeout:     30,
		Content:     instructions,
	}
	if err := s.skills.Save(sk); err != nil {
		L.Push(lua.LString("Error: " + err.Error()))
		return 1
	}
	// Create default main.lua script
	scriptsDir := filepath.Join(s.skills.Workspace(), "skills", name, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		L.Push(lua.LString("OK: skill created, but script dir failed: " + err.Error()))
		return 1
	}
	scriptContent := fmt.Sprintf(`-- %s skill
local function main(args)
    return "Skill '%s' executed with: " .. tostring(args)
end
return main
`, name, name)
	if err := os.WriteFile(filepath.Join(scriptsDir, "main.lua"), []byte(scriptContent), 0644); err != nil {
		L.Push(lua.LString("OK: skill created, but script failed: " + err.Error()))
		return 1
	}
	L.Push(lua.LString("OK: skill '" + name + "' created"))
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
		L.Push(lua.LString(`Available modules:
- file: File operations (file.read, file.write, file.edit, file.list)
- web: Web fetch and search (web.fetch, web.search)
- browser: Browser automation (browser.navigate, browser.click, browser.type, browser.screenshot)
- memory: Memory read/write/search (memory.today, memory.get, memory.write, memory.search)
- skill: Skill management (skill.list, skill.exec, skill.create)
- scheduler: Scheduled tasks (scheduler.add, scheduler.remove, scheduler.list)
- os: Date/time (os.date, os.time)
- time: Time utilities (time.time, time.date)

Use help("module") or help("module.function") for more info.`))
		return 1
	}

	module := L.Get(1).String()

	switch module {
	case "scheduler":
		L.Push(lua.LString(`scheduler - Manage scheduled tasks
scheduler.add(name, schedule, message)
scheduler.remove(name)
scheduler.list()`))
	case "file":
		L.Push(lua.LString(`file - File operations
file.read(path)     -> file content string
file.write(path, content) -> "OK" or error
file.edit(path, search, replace) -> "Edited" or error
file.list(path)     -> newline-separated filenames`))
	case "web":
		L.Push(lua.LString(`web - Web operations
web.fetch(url)      -> page content as string
web.search(query)   -> search results as string`))
	case "browser":
		L.Push(lua.LString(`browser - Browser automation (requires rod library)
browser.navigate(url)       -> "OK: navigated" or error
browser.click(selector)     -> "OK: clicked" or error
browser.type(selector, text) -> "OK: typed" or error
browser.screenshot()        -> "[screenshot: N bytes]" or error`))
	case "memory":
		L.Push(lua.LString(`memory - Memory operations
memory.today()              -> today's memory content
memory.get(ref-or-date)     -> memory by reference or date
memory.write(entry)         -> append entry to today's memory
memory.search(query)        -> grep memory files for query`))
	case "skill":
		L.Push(lua.LString(`skill - Skill management
skill.list()                -> list available skills
skill.exec(name, script, args?) -> execute a skill's Lua script
skill.create(name, desc?, instr?) -> create a new skill`))
	case "os":
		L.Push(lua.LString(`os - Operating system utilities (safe subset)
os.date() -> "YYYY-MM-DD HH:MM:SS"
os.time() -> Unix timestamp`))
	case "time":
		L.Push(lua.LString(`time - Time utilities (aliases for os.*)
time.time() -> Unix timestamp
time.date() -> "YYYY-MM-DD HH:MM:SS"`))
	default:
		L.Push(lua.LString("Unknown module: " + module + "\nUse help() to list all modules."))
	}
	return 1
}

func (s *Sandbox) hasFileOp(L *lua.LState) bool {
	if s.fileOp != nil {
		return true
	}
	L.Push(lua.LString("Error: file operations not available"))
	return false
}

func (s *Sandbox) hasWebOp(L *lua.LState) bool {
	if s.webOp != nil {
		return true
	}
	L.Push(lua.LString("Error: web operations not available"))
	return false
}

func (s *Sandbox) hasBrowserOp(L *lua.LState) bool {
	if s.browserOp != nil {
		return true
	}
	L.Push(lua.LString("Error: browser operations not available"))
	return false
}

func (s *Sandbox) hasMemoryOp(L *lua.LState) bool {
	if s.memoryOp != nil {
		return true
	}
	L.Push(lua.LString("Error: memory operations not available"))
	return false
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
