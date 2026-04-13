// Package shell provides shared shell resolution logic used by the exec
// extension and condition.Shell. It maps shell names (powershell, pwsh, cmd,
// bash, sh, auto) to binary paths and default parameters.
package shell

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Shell name constants.
const (
	Auto       = "auto"       // bash on Linux/macOS, powershell on Windows
	PowerShell = "powershell" // Windows PowerShell 5.1
	Pwsh       = "pwsh"       // PowerShell 7+
	Cmd        = "cmd"        // cmd.exe /C
	Bash       = "bash"       // /bin/bash -c
	Sh         = "sh"         // /bin/sh -c
)

const defaultPowerShellPath = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`

var defaultPSParams = []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass"}

const psPrefix = "$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'; "
const psSuffix = "; if ($LASTEXITCODE) { exit $LASTEXITCODE }"

// Resolve returns the binary path, parameter flags, and resolved shell name
// for the given shell and optional custom parameters.
func Resolve(shell string, customParams []string) (binary string, params []string, resolved string) {
	resolved = shell
	if resolved == Auto {
		if runtime.GOOS == "windows" {
			resolved = PowerShell
		} else {
			resolved = Bash
		}
	}

	if len(customParams) > 0 {
		params = customParams
	}

	switch resolved {
	case PowerShell:
		binary = defaultPowerShellPath
		if params == nil {
			params = defaultPSParams
		}
	case Pwsh:
		binary = "pwsh"
		if params == nil {
			params = defaultPSParams
		}
	case Cmd:
		binary = `C:\Windows\System32\cmd.exe`
	case Bash:
		binary = "/bin/bash"
	case Sh:
		binary = "/bin/sh"
	default:
		binary = resolved // custom path
	}
	return binary, params, resolved
}

// Command builds an exec.Cmd for the given shell and command string.
// The command is passed via -Command (PowerShell), /C (cmd), or -c (bash/sh).
func Command(ctx context.Context, shell, command string, customParams []string) *exec.Cmd {
	binary, params, resolved := Resolve(shell, customParams)
	args := make([]string, 0, len(params)+2)
	args = append(args, params...)

	switch resolved {
	case Bash, Sh:
		args = append(args, "-c", command)
	case Cmd:
		args = append(args, "/C", command)
	default:
		args = append(args, "-Command", psPrefix+command+psSuffix)
	}

	return exec.CommandContext(ctx, binary, args...)
}

// Run executes a shell command and returns its trimmed stdout.
func Run(ctx context.Context, shell, command string, customParams []string) (stdout string, err error) {
	cmd := Command(ctx, shell, command, customParams)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Truncate shortens a string for display in change descriptions.
// Trailing newlines and carriage returns are stripped before measuring.
func Truncate(s string, maxLen int) string {
	s = strings.TrimRight(s, "\n\r")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// IsPowerShell returns true if the resolved shell is a PowerShell variant.
func IsPowerShell(shell string) bool {
	resolved := shell
	if resolved == Auto && runtime.GOOS == "windows" {
		resolved = PowerShell
	}
	return resolved == PowerShell || resolved == Pwsh
}

// FormatOutput returns a human-readable description of command output for
// change reporting.
func FormatOutput(actual, expected string, err error) string {
	if actual == "" && err != nil {
		return fmt.Sprintf("(error: %v)", err)
	}
	return actual
}
