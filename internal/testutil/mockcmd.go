package testutil

import (
	"fmt"
	"os/exec"
	"sync"
)

// CmdCall records a single command invocation.
type CmdCall struct {
	Name string
	Args []string
}

// MockCmd records exec.Command calls and returns scripted results.
// Use it to replace exec.Command in extensions that shell out.
//
// Usage in extension:
//
//	var newCommand = exec.Command // production default
//
// Usage in test:
//
//	mock := testutil.NewMockCmd()
//	mock.SetOutput("modprobe", "", nil)  // modprobe succeeds silently
//	oldCmd := newCommand
//	newCommand = mock.Command
//	defer func() { newCommand = oldCmd }()
type MockCmd struct {
	mu      sync.Mutex
	Calls   []CmdCall
	outputs map[string]cmdResult
}

type cmdResult struct {
	stdout string
	err    error
}

// NewMockCmd creates a MockCmd with no scripted outputs.
// Unscripted commands return an error.
func NewMockCmd() *MockCmd {
	return &MockCmd{outputs: make(map[string]cmdResult)}
}

// SetOutput scripts the output for a command name.
// When Command is called with this name, the resulting exec.Cmd
// will produce stdout and exit with the given error.
func (m *MockCmd) SetOutput(name, stdout string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputs[name] = cmdResult{stdout: stdout, err: err}
}

// Command returns an exec.Cmd that, when run, records the call and
// returns the scripted output. It uses "echo" as the actual binary
// and encodes the result via environment variables, so CombinedOutput
// works without a real process.
//
// For simplicity, this returns an exec.Cmd that runs /bin/echo or
// cmd.exe /C echo with the scripted stdout. If the scripted error
// is non-nil, it returns a command that will fail.
func (m *MockCmd) Command(name string, args ...string) *exec.Cmd {
	m.mu.Lock()
	m.Calls = append(m.Calls, CmdCall{Name: name, Args: args})
	result, ok := m.outputs[name]
	m.mu.Unlock()

	if !ok {
		// Return a command that fails
		return exec.Command("false")
	}

	if result.err != nil {
		return exec.Command("false")
	}

	if result.stdout == "" {
		return exec.Command("true")
	}
	return exec.Command("echo", "-n", result.stdout)
}

// Called returns true if the named command was invoked.
func (m *MockCmd) Called(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.Calls {
		if c.Name == name {
			return true
		}
	}
	return false
}

// CallsFor returns all invocations of the named command.
func (m *MockCmd) CallsFor(name string) []CmdCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []CmdCall
	for _, c := range m.Calls {
		if c.Name == name {
			result = append(result, c)
		}
	}
	return result
}

// Reset clears all recorded calls and scripted outputs.
func (m *MockCmd) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = nil
	m.outputs = make(map[string]cmdResult)
}

// String returns a human-readable summary of all calls for debugging.
func (m *MockCmd) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var s string
	for i, c := range m.Calls {
		s += fmt.Sprintf("[%d] %s %v\n", i, c.Name, c.Args)
	}
	return s
}
