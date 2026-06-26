package condition

import (
	"context"
	"testing"
	"time"
)

// TestShell_Met_Timeout verifies a blocking command cannot hang Met()
// indefinitely: shellTimeout bounds it so the daemon (which evaluates conditions
// synchronously at startup) cannot be wedged by one slow command.
func TestShell_Met_Timeout(t *testing.T) {
	orig := shellTimeout
	shellTimeout = 200 * time.Millisecond
	defer func() { shellTimeout = orig }()

	start := time.Now()
	met, _ := Shell("sleep 5").Met(context.Background())
	elapsed := time.Since(start)

	if met {
		t.Error("Met() should be false when the command is killed by the timeout")
	}
	if elapsed > 3*time.Second {
		t.Errorf("Met() took %v; shellTimeout should have bounded it to ~200ms", elapsed)
	}
}

func TestShell_Met_ExitCode(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		command string
		wantMet bool
	}{
		{"exit 0 = met", "exit 0", true},
		{"exit 1 = not met", "exit 1", false},
		{"true = met", "true", true},
		{"false = not met", "false", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Shell(tt.command).In("bash")
			met, err := c.Met(ctx)
			if err != nil {
				t.Fatalf("Met() error = %v", err)
			}
			if met != tt.wantMet {
				t.Errorf("Met() = %v, want %v", met, tt.wantMet)
			}
		})
	}
}

func TestShell_Met_OutputMatch(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		command string
		expect  string
		wantMet bool
		wantErr bool
	}{
		{"output matches", "echo -n hello", "hello", true, false},
		{"output differs", "echo -n world", "hello", false, false},
		{"trailing newline trimmed", "echo hello", "hello", true, false},
		{"command fails, returns error", "false", "hello", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Shell(tt.command).In("bash").Match(tt.expect)
			met, err := c.Met(ctx)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Met() error = %v, wantErr %v", err, tt.wantErr)
			}
			if met != tt.wantMet {
				t.Errorf("Met() = %v, want %v", met, tt.wantMet)
			}
		})
	}
}

func TestShell_String(t *testing.T) {
	tests := []struct {
		name string
		cond *shellCondition
		want string
	}{
		{"with match", Shell("echo hello").In("bash").Match("hello"), `shell bash: echo hello == "hello"`},
		{"no match", Shell("pgrep nginx").In("bash"), "shell bash: pgrep nginx"},
		{"auto shell", Shell("pgrep nginx"), "shell auto: pgrep nginx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cond.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShell_ChainOrder(t *testing.T) {
	ctx := context.Background()

	// .In().Match() and .Match().In() should produce the same result
	a := Shell("echo -n yes").In("bash").Match("yes")
	b := Shell("echo -n yes").Match("yes").In("bash")

	metA, _ := a.Met(ctx)
	metB, _ := b.Met(ctx)

	if metA != metB {
		t.Errorf("chain order matters: .In().Match()=%v, .Match().In()=%v", metA, metB)
	}
	if !metA {
		t.Error("both should be met")
	}
}

func TestShell_MultilineScript(t *testing.T) {
	ctx := context.Background()

	c := Shell("x=hello\necho -n $x").In("bash").Match("hello")
	met, _ := c.Met(ctx)
	if !met {
		t.Error("multi-line bash script should match output")
	}
}

func TestShell_DefaultShellIsAuto(t *testing.T) {
	c := Shell("echo test")
	if c.shellName != "auto" {
		t.Errorf("default shell = %q, want %q", c.shellName, "auto")
	}
}
