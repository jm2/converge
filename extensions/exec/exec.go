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

	// Idempotency guards. With none set, Check always reports out-of-sync and
	// the command runs on every convergence. A guard short-circuits Check to
	// in-sync so the command is skipped.
	Creates string // skip if this path exists
	OnlyIf  string // skip unless this command succeeds (exit 0)
	Unless  string // skip if this command succeeds (exit 0)
}

// Exec runs an arbitrary command. Use the Creates/OnlyIf/Unless guards to make
// it idempotent; without a guard it runs on every convergence.
//
// When Shell is set, Command is wrapped: the shell binary is invoked with
// default flags (or ShellParams) followed by the command. Command can be
// multi-line. Shell and Args are mutually exclusive.
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

	Creates string
	OnlyIf  string
	Unless  string
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
		Creates:     opts.Creates,
		OnlyIf:      opts.OnlyIf,
		Unless:      opts.Unless,
	}
}

func (e *Exec) ID() string       { return fmt.Sprintf("exec:%s", e.Name) }
func (e *Exec) String() string   { return fmt.Sprintf("Exec %s", e.Name) }
func (e *Exec) IsCritical() bool { return e.Critical }

// AlwaysApplies reports whether this Exec has no idempotency guard, in which
// case Check always reports out-of-sync and the command runs on every
// convergence. The engine uses this to skip the post-Apply re-Check, which
// would otherwise flag a successful guardless run as "still out of sync after
// apply". A guarded Exec is convergent (the guard short-circuits Check once
// satisfied), so it returns false and is verified normally.
func (e *Exec) AlwaysApplies() bool {
	return e.Creates == "" && e.OnlyIf == "" && e.Unless == ""
}

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

// validate rejects mutually exclusive configuration. Shell wraps Command in a
// shell invocation, leaving no place for Args, so the two cannot be combined.
func (e *Exec) validate() error {
	if e.Shell != "" && len(e.Args) > 0 {
		return fmt.Errorf("exec %s: Shell and Args are mutually exclusive", e.Name)
	}
	return nil
}

// runGuard executes a guard command and returns its exit status as an error
// (nil on exit 0). Guard commands always run through a shell since they are
// command strings, defaulting to the auto shell when none is configured.
func (e *Exec) runGuard(ctx context.Context, command string) error {
	sh := e.Shell
	if sh == "" {
		sh = shell.Auto
	}
	cmd := shell.Command(ctx, sh, command, e.ShellParams)
	e.setEnv(cmd)
	return cmd.Run()
}

// Check evaluates the idempotency guards. With no guard set it always reports
// out-of-sync so the command runs. Otherwise a guard can short-circuit to
// in-sync: Creates when the path exists, OnlyIf when its command fails, and
// Unless when its command succeeds.
func (e *Exec) Check(ctx context.Context) (*extensions.State, error) {
	if err := e.validate(); err != nil {
		return nil, err
	}

	inSync := &extensions.State{InSync: true}

	if e.Creates != "" {
		if _, err := os.Stat(e.Creates); err == nil {
			return inSync, nil
		}
	}
	if e.OnlyIf != "" {
		if err := e.runGuard(ctx, e.OnlyIf); err != nil {
			return inSync, nil
		}
	}
	if e.Unless != "" {
		if err := e.runGuard(ctx, e.Unless); err == nil {
			return inSync, nil
		}
	}

	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{
			{Property: "command", To: e.Command, Action: "add"},
		},
	}, nil
}

// maxErrOutput caps how much command output is embedded in an Apply error, to
// avoid leaking large or sensitive output into logs and JSON.
const maxErrOutput = 500

// tailOutput returns the trailing maxErrOutput bytes of command output for
// inclusion in an error message.
func tailOutput(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > maxErrOutput {
		return "..." + s[len(s)-maxErrOutput:]
	}
	return s
}

func (e *Exec) Apply(ctx context.Context) (*extensions.Result, error) {
	if err := e.validate(); err != nil {
		return nil, err
	}

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

		// Do not embed the raw command (may contain secrets); truncate output.
		lastErr = fmt.Errorf("command failed (attempt %d/%d): %s: %w", attempt+1, retries, tailOutput(output), err)
		if attempt < retries-1 {
			time.Sleep(delay)
		}
	}

	return nil, lastErr
}
