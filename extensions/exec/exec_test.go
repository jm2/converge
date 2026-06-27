package exec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/TsekNet/converge/internal/shell"
)

// guardCommands returns shell command strings that always succeed and always
// fail on the current platform. Guards run through the auto shell (bash/sh on
// Unix, PowerShell on Windows). The /bin/true and /bin/false binaries are
// unavailable on macOS (no /bin/true or /bin/false) and Windows, so use the
// shell builtins on Unix and explicit exit codes under PowerShell.
func guardCommands() (succeed, fail string) {
	if runtime.GOOS == "windows" {
		return "exit 0", "exit 1"
	}
	return "true", "false"
}

func TestExec_Check_NoGuards(t *testing.T) {
	ctx := context.Background()
	e := New("test", Opts{Command: "echo"})

	state, err := e.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("Exec.Check with no guards should report not-in-sync (command runs)")
	}
}

func TestExec_Check_Creates(t *testing.T) {
	ctx := context.Background()

	existing := filepath.Join(t.TempDir(), "marker")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "absent")

	tests := []struct {
		name       string
		path       string
		wantInSync bool
	}{
		{"path exists -> in sync", existing, true},
		{"path missing -> runs", missing, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New("test", Opts{Command: "echo", Creates: tt.path})
			state, err := e.Check(ctx)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if state.InSync != tt.wantInSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantInSync)
			}
		})
	}
}

func TestExec_Check_OnlyIf(t *testing.T) {
	ctx := context.Background()

	// OnlyIf: run only when the guard command succeeds. When it fails, the
	// resource is skipped (reported in sync).
	succeed, fail := guardCommands()
	tests := []struct {
		name       string
		guard      string
		wantInSync bool
	}{
		{"guard succeeds -> runs", succeed, false},
		{"guard fails -> skip", fail, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New("test", Opts{Command: "echo", OnlyIf: tt.guard})
			state, err := e.Check(ctx)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if state.InSync != tt.wantInSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantInSync)
			}
		})
	}
}

func TestExec_Check_Unless(t *testing.T) {
	ctx := context.Background()

	// Unless: skip when the guard command succeeds; run when it fails.
	succeed, fail := guardCommands()
	tests := []struct {
		name       string
		guard      string
		wantInSync bool
	}{
		{"guard succeeds -> skip", succeed, true},
		{"guard fails -> runs", fail, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New("test", Opts{Command: "echo", Unless: tt.guard})
			state, err := e.Check(ctx)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if state.InSync != tt.wantInSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantInSync)
			}
		})
	}
}

func TestExec_ShellAndArgs_Error(t *testing.T) {
	ctx := context.Background()
	e := New("test", Opts{Command: "echo hi", Shell: shell.Bash, Args: []string{"dropped"}})

	if _, err := e.Check(ctx); err == nil {
		t.Error("Check should error when both Shell and Args are set")
	}
	if _, err := e.Apply(ctx); err == nil {
		t.Error("Apply should error when both Shell and Args are set")
	}
}

func TestExec_Apply_ErrorOmitsCommandAndTruncates(t *testing.T) {
	ctx := context.Background()

	// Output longer than maxErrOutput must be truncated, and the secret-bearing
	// command string must not appear in the error.
	long := strings.Repeat("z", maxErrOutput+200)
	e := New("test", Opts{
		Command: "printf '" + long + "'; exit 1",
		Shell:   shell.Bash,
	})

	_, err := e.Apply(ctx)
	if err == nil {
		t.Fatal("expected error from failing command")
	}
	msg := err.Error()
	if strings.Contains(msg, e.Command) {
		t.Error("error must not embed the raw command")
	}
	if strings.Count(msg, "z") > maxErrOutput {
		t.Errorf("output not truncated: %d z's in error", strings.Count(msg, "z"))
	}
}

func TestExec_Apply(t *testing.T) {
	ctx := context.Background()

	// "with working directory" runs a bare (non-shell) command inside Dir. Use a
	// platform-appropriate always-succeed command and a real temp dir, since
	// /tmp and a bare `pwd`/`false` binary do not exist on Windows.
	wdDir := t.TempDir()
	wdExec := &Exec{Name: "pwd", Command: "pwd", Dir: wdDir}
	falseExec := New("fail", Opts{Command: "false"})
	if runtime.GOOS == "windows" {
		wdExec = &Exec{Name: "pwd", Command: "cmd", Args: []string{"/c", "cd"}, Dir: wdDir}
		falseExec = New("fail", Opts{Command: "cmd", Args: []string{"/c", "exit 1"}})
	}

	tests := []struct {
		name    string
		exec    *Exec
		wantErr bool
	}{
		{
			"simple echo",
			New("echo", Opts{Command: "echo", Args: []string{"hello"}}),
			false,
		},
		{
			"command not found",
			New("bad", Opts{Command: "/nonexistent/binary"}),
			true,
		},
		{
			"false command fails",
			falseExec,
			true,
		},
		{
			"with working directory",
			wdExec,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.exec.Apply(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && !result.Changed {
				t.Error("Changed should be true on success")
			}
		})
	}
}

func TestExec_Apply_Shell(t *testing.T) {
	ctx := context.Background()

	// /bin/bash and /bin/sh do not exist on Windows; exercise the platform
	// shell (PowerShell) there instead of the POSIX shells.
	tests := []struct {
		name    string
		shell   string
		command string
		wantErr bool
	}{
		{"bash echo", shell.Bash, "echo hello", false},
		{"sh echo", shell.Sh, "echo hello", false},
		{"bash multiline", shell.Bash, "x=42\necho $x", false},
		{"bash failure", shell.Bash, "exit 1", true},
	}
	if runtime.GOOS == "windows" {
		tests = []struct {
			name    string
			shell   string
			command string
			wantErr bool
		}{
			{"powershell echo", shell.PowerShell, "echo hello", false},
			{"powershell multiline", shell.PowerShell, "$x=42\necho $x", false},
			{"powershell failure", shell.PowerShell, "exit 1", true},
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New("test", Opts{Command: tt.command, Shell: tt.shell})
			result, err := e.Apply(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("Apply() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && !result.Changed {
				t.Error("Changed should be true on success")
			}
		})
	}
}

func TestExec_Apply_ShellParams(t *testing.T) {
	ctx := context.Background()

	// Custom ShellParams replace the default shell flags. Use a platform shell
	// and a flag it accepts: bash trace mode on Unix, PowerShell flags on
	// Windows (where /bin/bash is unavailable).
	sh, params := shell.Bash, []string{"-x"} // trace mode
	if runtime.GOOS == "windows" {
		sh, params = shell.PowerShell, []string{"-NoProfile", "-NonInteractive"}
	}
	e := New("test", Opts{
		Command:     "echo custom",
		Shell:       sh,
		ShellParams: params,
	})
	result, err := e.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Changed {
		t.Error("Changed should be true")
	}
}

func TestExec_ApplyRetries(t *testing.T) {
	ctx := context.Background()

	e := &Exec{
		Name:       "retry-test",
		Command:    "false",
		Retries:    3,
		RetryDelay: 10 * time.Millisecond,
	}

	start := time.Now()
	_, err := e.Apply(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from false command")
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("retries should have taken at least 20ms, took %v", elapsed)
	}
}

func TestExec_IDAndString(t *testing.T) {
	tests := []struct {
		name    string
		wantID  string
		wantStr string
	}{
		{"setup-fw", "exec:setup-fw", "Exec setup-fw"},
		{"install-deps", "exec:install-deps", "Exec install-deps"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New(tt.name, Opts{Command: "echo"})
			if e.ID() != tt.wantID {
				t.Errorf("ID() = %q, want %q", e.ID(), tt.wantID)
			}
			if e.String() != tt.wantStr {
				t.Errorf("String() = %q, want %q", e.String(), tt.wantStr)
			}
		})
	}
}

func TestExec_IsCritical(t *testing.T) {
	e := New("test", Opts{Command: "echo"})
	if e.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	e2 := New("test", Opts{Command: "echo", Critical: true})
	if !e2.IsCritical() {
		t.Error("IsCritical() should be true when set")
	}
}
