package exec

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"time"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/shell"
)

// Opts configures an Exec resource.
type Opts struct {
	Command     string
	Args        []string
	Shell       string   // "powershell", "pwsh", "cmd", "bash", "sh", "auto", or custom path
	ShellParams []string // when set, replaces default shell flags
	Dir         string
	Env         []string
	Retries     int
	RetryDelay  time.Duration
	Critical    bool
}

// Exec runs an arbitrary command. For state detection, use condition.Shell
// on Meta.Condition instead of guards on the Exec resource itself.
//
// When Shell is set, Command is wrapped: the shell binary is invoked with
// default flags (or ShellParams) followed by the command. Command can be
// multi-line.
type Exec struct {
	Name        string
	Command     string
	Args        []string
	Shell       string
	ShellParams []string
	Dir         string
	Env         []string
	Retries     int
	RetryDelay  time.Duration
	Critical    bool
}

func New(name string, opts Opts) *Exec {
	return &Exec{
		Name:        name,
		Command:     opts.Command,
		Args:        opts.Args,
		Shell:       opts.Shell,
		ShellParams: opts.ShellParams,
		Dir:         opts.Dir,
		Env:         opts.Env,
		Retries:     opts.Retries,
		RetryDelay:  opts.RetryDelay,
		Critical:    opts.Critical,
	}
}

func (e *Exec) ID() string       { return fmt.Sprintf("exec:%s", e.Name) }
func (e *Exec) String() string   { return fmt.Sprintf("Exec %s", e.Name) }
func (e *Exec) IsCritical() bool { return e.Critical }

// applyCmd builds an exec.Cmd for the Apply command.
func (e *Exec) applyCmd(ctx context.Context) *osexec.Cmd {
	if e.Shell != "" {
		return shell.Command(ctx, e.Shell, e.Command, e.ShellParams)
	}
	return osexec.CommandContext(ctx, e.Command, e.Args...)
}

func (e *Exec) setEnv(cmd *osexec.Cmd) {
	if e.Dir != "" {
		cmd.Dir = e.Dir
	}
	if len(e.Env) > 0 {
		cmd.Env = append(os.Environ(), e.Env...)
	}
}

// Check for Exec always returns not-in-sync (the command needs to run).
// Use condition.Shell on Meta.Condition for state detection.
func (e *Exec) Check(_ context.Context) (*extensions.State, error) {
	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{
			{Property: "command", To: e.Command, Action: "add"},
		},
	}, nil
}

func (e *Exec) Apply(ctx context.Context) (*extensions.Result, error) {
	retries := e.Retries
	if retries <= 0 {
		retries = 1
	}
	delay := e.RetryDelay
	if delay <= 0 {
		delay = time.Second
	}

	var lastErr error
	for attempt := range retries {
		cmd := e.applyCmd(ctx)
		e.setEnv(cmd)

		output, err := cmd.CombinedOutput()
		if err == nil {
			return &extensions.Result{
				Changed: true,
				Status:  extensions.StatusChanged,
				Message: "executed",
			}, nil
		}

		lastErr = fmt.Errorf("%s (attempt %d/%d): %s: %w", e.Command, attempt+1, retries, strings.TrimSpace(string(output)), err)
		if attempt < retries-1 {
			time.Sleep(delay)
		}
	}

	return nil, lastErr
}
