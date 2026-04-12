package exec

import (
	"context"
	"testing"
	"time"
)

func TestExec_Check(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		onlyIf   string
		wantSync bool
	}{
		{"no guard command", "", false},
		{"guard passes (true)", "true", true},
		{"guard fails (false)", "false", false},
		{"guard with args", "test -f /etc/os-release", true},
		{"guard file missing", "test -f /nonexistent/file", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Exec{Name: "test", Command: "echo", OnlyIf: tt.onlyIf}
			state, err := e.Check(ctx)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if state.InSync != tt.wantSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantSync)
			}
		})
	}
}

func TestExec_Check_OnlyIfMatch(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		onlyIf      string
		onlyIfMatch string
		wantSync    bool
	}{
		{"output matches", "echo -n hello", "hello", true},
		{"output differs", "echo -n world", "hello", false},
		{"output with trailing newline", "echo hello", "hello", true},
		{"command fails, no match", "false", "hello", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Exec{
				Name:        "test",
				Command:     "echo",
				OnlyIf:      tt.onlyIf,
				OnlyIfMatch: tt.onlyIfMatch,
			}
			state, err := e.Check(ctx)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if state.InSync != tt.wantSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantSync)
			}
		})
	}
}

func TestExec_Check_ShellBash(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		onlyIf   string
		wantSync bool
	}{
		{"bash guard passes", "exit 0", true},
		{"bash guard fails", "exit 1", false},
		{"bash multiline", "x=hello\necho $x\nexit 0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Exec{
				Name:    "test",
				Command: "echo done",
				OnlyIf:  tt.onlyIf,
				Shell:   ShellBash,
			}
			state, err := e.Check(ctx)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if state.InSync != tt.wantSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantSync)
			}
		})
	}
}

func TestExec_Check_ShellBash_OnlyIfMatch(t *testing.T) {
	ctx := context.Background()

	e := &Exec{
		Name:        "test",
		Command:     "echo done",
		OnlyIf:      "echo -n myvalue",
		OnlyIfMatch: "myvalue",
		Shell:       ShellBash,
	}
	state, err := e.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !state.InSync {
		t.Errorf("should be in sync when bash output matches OnlyIfMatch")
	}
}

func TestExec_Apply(t *testing.T) {
	ctx := context.Background()

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
			New("fail", Opts{Command: "false"}),
			true,
		},
		{
			"with working directory",
			&Exec{Name: "pwd", Command: "pwd", Dir: "/tmp"},
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

	tests := []struct {
		name    string
		shell   string
		command string
		wantErr bool
	}{
		{"bash echo", ShellBash, "echo hello", false},
		{"sh echo", ShellSh, "echo hello", false},
		{"bash multiline", ShellBash, "x=42\necho $x", false},
		{"bash failure", ShellBash, "exit 1", true},
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

	e := New("test", Opts{
		Command:     "echo custom",
		Shell:       ShellBash,
		ShellParams: []string{"-x"}, // trace mode
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

func TestResolveShell(t *testing.T) {
	tests := []struct {
		shell      string
		wantBinary string
	}{
		{ShellPowerShell, defaultPowerShellPath},
		{ShellPwsh, "pwsh"},
		{ShellCmd, `C:\Windows\System32\cmd.exe`},
		{ShellBash, "/bin/bash"},
		{ShellSh, "/bin/sh"},
		{"/usr/local/bin/zsh", "/usr/local/bin/zsh"},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			e := &Exec{Shell: tt.shell}
			binary, _ := e.resolveShell()
			if binary != tt.wantBinary {
				t.Errorf("binary = %q, want %q", binary, tt.wantBinary)
			}
		})
	}
}

func TestResolveShell_CustomParams(t *testing.T) {
	e := &Exec{
		Shell:       ShellPowerShell,
		ShellParams: []string{"-ExecutionPolicy", "RemoteSigned"},
	}
	_, params := e.resolveShell()
	if len(params) != 2 || params[0] != "-ExecutionPolicy" {
		t.Errorf("params = %v, want custom params", params)
	}
}

func TestResolvedShell_Auto(t *testing.T) {
	e := &Exec{Shell: ShellAuto}
	got := e.resolvedShell()
	// On Linux (CI), auto resolves to bash
	if got != ShellBash && got != ShellPowerShell {
		t.Errorf("resolvedShell() = %q, want bash or powershell", got)
	}
}

func TestResolveShell_DefaultPSParams(t *testing.T) {
	e := &Exec{Shell: ShellPowerShell}
	_, params := e.resolveShell()
	if len(params) != len(defaultPSParams) {
		t.Errorf("params = %v, want default PS params %v", params, defaultPSParams)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"this is way too long", 10, "this is wa..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := truncate(tt.input, tt.max); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}
