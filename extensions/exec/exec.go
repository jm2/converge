package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/TsekNet/converge/extensions"
)

// Shell constants for the Shell field.
const (
	ShellAuto       = "auto"       // bash on Linux/macOS, powershell on Windows
	ShellPowerShell = "powershell" // Windows PowerShell 5.1 (C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe)
	ShellPwsh       = "pwsh"       // PowerShell 7+ (pwsh.exe from PATH)
	ShellCmd        = "cmd"        // cmd.exe /C
	ShellBash       = "bash"       // /bin/bash -c
	ShellSh         = "sh"         // /bin/sh -c
)

// defaultPowerShellPath is the full path to Windows PowerShell 5.1.
const defaultPowerShellPath = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`

// defaultPSParams are the default flags for PowerShell invocations.
var defaultPSParams = []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass"}

// psPrefix is prepended to every PowerShell -Command invocation.
// $ErrorActionPreference = 'Stop': converts non-terminating errors to terminating,
// preventing silent failures from cmdlets that print an error but exit 0.
// $ProgressPreference = 'SilentlyContinue': suppresses Write-Progress output that
// corrupts stdout parsing for OnlyIfMatch and adds latency.
const psPrefix = "$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'; "

// psSuffix is appended to every PowerShell -Command invocation.
// Propagates $LASTEXITCODE from native executables (msiexec, reg.exe, etc.)
// to the process exit code. Without this, powershell.exe exits 0 even when
// the native exe fails. Only fires when $LASTEXITCODE is non-null (pure
// cmdlet commands never set it).
const psSuffix = "; if ($LASTEXITCODE) { exit $LASTEXITCODE }"

// Opts configures an Exec resource.
type Opts struct {
	Command     string
	Args        []string
	OnlyIf      string   // guard: exit 0 = in sync (or output match if OnlyIfMatch set)
	OnlyIfMatch string   // when set, OnlyIf compares trimmed stdout against this string
	Shell       string   // "powershell", "pwsh", "bash", "sh", or "" (direct exec)
	ShellParams []string // when set, replaces default shell flags (not the binary or -Command/-c)
	Dir         string
	Env         []string
	Retries     int
	RetryDelay  time.Duration
	Critical    bool
}

// Exec runs an arbitrary command. Use OnlyIf as a guard: if the guard exits 0
// (or stdout matches OnlyIfMatch), the resource is already in sync.
//
// When Shell is set, Command is wrapped: the shell binary is invoked with
// default flags (or ShellParams) followed by the command. Command can be
// multi-line.
type Exec struct {
	Name        string
	Command     string
	Args        []string
	OnlyIf      string
	OnlyIfMatch string
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
		OnlyIf:      opts.OnlyIf,
		OnlyIfMatch: opts.OnlyIfMatch,
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

// shellCmd builds an exec.Cmd for the given command string, respecting the
// Shell setting. When Shell is empty, the command is split on whitespace
// (for guards) or used directly with Args (for Apply).
func (e *Exec) shellCmd(ctx context.Context, command string) *exec.Cmd {
	if e.Shell == "" {
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return exec.CommandContext(ctx, command)
		}
		return exec.CommandContext(ctx, parts[0], parts[1:]...)
	}

	binary, params, shell := e.resolveShell()
	args := make([]string, 0, len(params)+2)
	args = append(args, params...)

	switch shell {
	case ShellBash, ShellSh:
		args = append(args, "-c", command)
	case ShellCmd:
		args = append(args, "/C", command)
	default:
		// PowerShell variants: wrap with error/progress preferences
		args = append(args, "-Command", psPrefix+command+psSuffix)
	}

	return exec.CommandContext(ctx, binary, args...)
}

// resolveShell returns the binary path, parameter flags, and resolved shell
// name for the configured shell. The resolved name accounts for "auto"
// expanding to the platform default.
func (e *Exec) resolveShell() (binary string, params []string, shell string) {
	if len(e.ShellParams) > 0 {
		params = e.ShellParams
	}

	shell = e.Shell
	if shell == ShellAuto {
		if runtime.GOOS == "windows" {
			shell = ShellPowerShell
		} else {
			shell = ShellBash
		}
	}

	switch shell {
	case ShellPowerShell:
		binary = defaultPowerShellPath
		if params == nil {
			params = defaultPSParams
		}
	case ShellPwsh:
		binary = "pwsh"
		if params == nil {
			params = defaultPSParams
		}
	case ShellCmd:
		binary = `C:\Windows\System32\cmd.exe`
	case ShellBash:
		binary = "/bin/bash"
	case ShellSh:
		binary = "/bin/sh"
	default:
		binary = shell // allow arbitrary shell path
	}
	return binary, params, shell
}

// applyCmd builds an exec.Cmd for the Apply command.
func (e *Exec) applyCmd(ctx context.Context) *exec.Cmd {
	if e.Shell != "" {
		return e.shellCmd(ctx, e.Command)
	}
	return exec.CommandContext(ctx, e.Command, e.Args...)
}

func (e *Exec) setEnv(cmd *exec.Cmd) {
	if e.Dir != "" {
		cmd.Dir = e.Dir
	}
	if len(e.Env) > 0 {
		cmd.Env = append(os.Environ(), e.Env...)
	}
}

// checkGuard runs the OnlyIf command. When OnlyIfMatch is empty, exit 0 = in sync.
// When OnlyIfMatch is set, trimmed stdout must equal OnlyIfMatch for in-sync.
func (e *Exec) checkGuard(ctx context.Context) (*extensions.State, error) {
	if e.OnlyIf == "" {
		return &extensions.State{InSync: false}, nil
	}

	cmd := e.shellCmd(ctx, e.OnlyIf)
	e.setEnv(cmd)

	if e.OnlyIfMatch != "" {
		return e.checkGuardOutput(cmd)
	}

	err := cmd.Run()
	if err == nil {
		return &extensions.State{InSync: true}, nil
	}

	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{
			{Property: "command", To: e.Command, Action: "add"},
		},
	}, nil
}

// checkGuardOutput runs the guard and compares trimmed stdout to OnlyIfMatch.
func (e *Exec) checkGuardOutput(cmd *exec.Cmd) (*extensions.State, error) {
	out, err := cmd.Output()
	// Even if exit code is non-zero, check output: some PS commands
	// return the right value but exit non-zero.
	actual := strings.TrimSpace(string(out))

	if err == nil && actual == e.OnlyIfMatch {
		return &extensions.State{InSync: true}, nil
	}

	from := actual
	if from == "" && err != nil {
		from = fmt.Sprintf("(error: %v)", err)
	}

	return &extensions.State{
		InSync: false,
		Changes: []extensions.Change{
			{Property: "output", From: truncate(from, 60), To: e.OnlyIfMatch, Action: "modify"},
		},
	}, nil
}

func (e *Exec) Check(ctx context.Context) (*extensions.State, error) {
	return e.checkGuard(ctx)
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
