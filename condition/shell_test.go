package condition

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/TsekNet/converge/internal/shell"
)

// shellCmds holds platform-appropriate command strings so the cross-platform
// exit-code and output-match behavior of condition.Shell can be exercised on
// both Unix (bash) and Windows (PowerShell) without hardcoding bash syntax.
// Fields left empty have no straightforward equivalent on that platform and
// the corresponding case is skipped.
type shellCmds struct {
	exitZero    string // exits 0
	exitOne     string // exits 1 (also used to assert command-failure errors)
	trueCmd     string // builtin/command that exits 0 ("" if none)
	falseCmd    string // builtin/command that exits 1 ("" if none)
	echoHello   string // prints "hello" (no trailing newline where possible)
	echoHelloNL string // prints "hello" with a trailing newline
	echoWorld   string // prints "world"
	echoData    string // prints "data"
	echoYes     string // prints "yes"
	empty       string // produces empty output, exits 0
	multiline   string // multi-line script that prints "hello"
}

// platformCmds returns command strings for the platform-default shell
// (PowerShell on Windows, bash on Unix). condition.Shell defaults to
// shell.Auto, so these run under the OS-appropriate shell.
func platformCmds() shellCmds {
	if runtime.GOOS == "windows" {
		return shellCmds{
			exitZero: "exit 0",
			exitOne:  "exit 1",
			// PowerShell has no `true`/`false` builtins; those cases skip.
			echoHello:   "Write-Output hello",
			echoHelloNL: "Write-Output hello",
			echoWorld:   "Write-Output world",
			echoData:    "Write-Output data",
			echoYes:     "Write-Output yes",
			empty:       "Write-Output ''",
			multiline:   "$x = 'hello'\nWrite-Output $x",
		}
	}
	return shellCmds{
		exitZero:    "exit 0",
		exitOne:     "exit 1",
		trueCmd:     "true",
		falseCmd:    "false",
		echoHello:   "echo -n hello",
		echoHelloNL: "echo hello",
		echoWorld:   "echo -n world",
		echoData:    "echo -n data",
		echoYes:     "echo -n yes",
		empty:       `printf ''`,
		multiline:   "x=hello\necho -n $x",
	}
}

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
	cmds := platformCmds()

	tests := []struct {
		name    string
		command string
		wantMet bool
	}{
		{"exit 0 = met", cmds.exitZero, true},
		{"exit 1 = not met", cmds.exitOne, false},
		{"true = met", cmds.trueCmd, true},
		{"false = not met", cmds.falseCmd, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.command == "" {
				t.Skip("no equivalent command on this platform")
			}
			// Default shell (shell.Auto): bash on Unix, PowerShell on Windows.
			c := Shell(tt.command)
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
	cmds := platformCmds()

	tests := []struct {
		name    string
		command string
		expect  string
		wantMet bool
		wantErr bool
	}{
		{"output matches", cmds.echoHello, "hello", true, false},
		{"output differs", cmds.echoWorld, "hello", false, false},
		{"trailing newline trimmed", cmds.echoHelloNL, "hello", true, false},
		{"command fails, returns error", cmds.exitOne, "hello", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Shell(tt.command).Match(tt.expect)
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

func TestShell_Met_MatchEmpty(t *testing.T) {
	ctx := context.Background()
	cmds := platformCmds()

	t.Run("empty output asserted via Match(\"\")", func(t *testing.T) {
		// A command with no output; Match("") must assert that.
		c := Shell(cmds.empty).Match("")
		met, err := c.Met(ctx)
		if err != nil {
			t.Fatalf("Met() error = %v", err)
		}
		if !met {
			t.Error("Match(\"\") should be met when output is empty")
		}
	})

	t.Run("non-empty output fails Match(\"\")", func(t *testing.T) {
		c := Shell(cmds.echoData).Match("")
		met, err := c.Met(ctx)
		if err != nil {
			t.Fatalf("Met() error = %v", err)
		}
		if met {
			t.Error("Match(\"\") should not be met when output is non-empty")
		}
	})

	t.Run("without Match, exit code governs", func(t *testing.T) {
		// No Match call: empty output but exit 0 is still met.
		c := Shell(cmds.empty)
		met, err := c.Met(ctx)
		if err != nil {
			t.Fatalf("Met() error = %v", err)
		}
		if !met {
			t.Error("exit 0 without Match should be met regardless of output")
		}
	})
}

func TestShell_Met_MatchTrimsBothSides(t *testing.T) {
	ctx := context.Background()
	cmds := platformCmds()

	// Expected value carries surrounding whitespace; it must be trimmed too,
	// so it still matches the (trimmed) command output.
	c := Shell(cmds.echoHelloNL).Match("  hello\n")
	met, err := c.Met(ctx)
	if err != nil {
		t.Fatalf("Met() error = %v", err)
	}
	if !met {
		t.Error("Match should trim both expected and actual before comparing")
	}
}

func TestShell_String(t *testing.T) {
	tests := []struct {
		name string
		cond *shellCondition
		want string
	}{
		{"with match", Shell("echo hello").In("bash").Match("hello"), `shell bash: echo hello == "hello"`},
		{"empty match", Shell("echo hello").In("bash").Match(""), `shell bash: echo hello == ""`},
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
	cmds := platformCmds()

	// .In().Match() and .Match().In() should produce the same result. Use
	// shell.Auto so the script runs under the OS-appropriate shell.
	a := Shell(cmds.echoYes).In(shell.Auto).Match("yes")
	b := Shell(cmds.echoYes).Match("yes").In(shell.Auto)

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
	cmds := platformCmds()

	// Multi-line script in the platform-default shell should match output.
	c := Shell(cmds.multiline).Match("hello")
	met, _ := c.Met(ctx)
	if !met {
		t.Error("multi-line script should match output")
	}
}

func TestShell_DefaultShellIsAuto(t *testing.T) {
	c := Shell("echo test")
	if c.shellName != "auto" {
		t.Errorf("default shell = %q, want %q", c.shellName, "auto")
	}
}
