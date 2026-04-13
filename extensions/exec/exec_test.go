package exec

import (
	"context"
	"testing"
	"time"

	"github.com/TsekNet/converge/internal/shell"
)

func TestExec_Check_AlwaysNotInSync(t *testing.T) {
	ctx := context.Background()
	e := New("test", Opts{Command: "echo"})

	state, err := e.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("Exec.Check should always return not-in-sync (use condition.Shell for guards)")
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
		{"bash echo", shell.Bash, "echo hello", false},
		{"sh echo", shell.Sh, "echo hello", false},
		{"bash multiline", shell.Bash, "x=42\necho $x", false},
		{"bash failure", shell.Bash, "exit 1", true},
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
		Shell:       shell.Bash,
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
