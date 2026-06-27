package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of fn and returns whatever
// was written. The CLI's leaf commands (version, list) print with fmt.Print*
// straight to os.Stdout rather than the cobra command's writer, so intercepting
// the file descriptor is the only way to observe them.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

// execCmd runs the root command with the given args and returns combined
// behavior: captured stdout and the error Execute reports. It restores the
// shared rootCmd arg state afterward so other tests are unaffected.
func execCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var execErr error
	out := captureStdout(t, func() {
		rootCmd.SetArgs(args)
		execErr = rootCmd.Execute()
	})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	return out, execErr
}

func TestVersionCommand(t *testing.T) {
	out, err := execCmd(t, "version")
	if err != nil {
		t.Fatalf("version command: %v", err)
	}
	for _, want := range []string{"converge ", "commit:", "built:", "go:", "os:"} {
		if !strings.Contains(out, want) {
			t.Errorf("version output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestListCommandDefault(t *testing.T) {
	out, err := execCmd(t, "list")
	if err != nil {
		t.Fatalf("list command: %v", err)
	}
	// Default (no flags) lists both sections.
	if !strings.Contains(out, "Extensions:") {
		t.Errorf("list output missing Extensions section\ngot:\n%s", out)
	}
	if !strings.Contains(out, "Blueprints:") {
		t.Errorf("list output missing Blueprints section\ngot:\n%s", out)
	}
}

func TestListCommandExtensionsOnly(t *testing.T) {
	defer func() { listExtensions = false; listBlueprints = false }()
	out, err := execCmd(t, "list", "--extensions")
	if err != nil {
		t.Fatalf("list --extensions: %v", err)
	}
	if !strings.Contains(out, "Extensions:") {
		t.Errorf("missing Extensions section\ngot:\n%s", out)
	}
	if strings.Contains(out, "Blueprints:") {
		t.Errorf("--extensions should not print Blueprints section\ngot:\n%s", out)
	}
}

func TestListCommandBlueprintsOnly(t *testing.T) {
	defer func() { listExtensions = false; listBlueprints = false }()
	out, err := execCmd(t, "list", "--blueprints")
	if err != nil {
		t.Fatalf("list --blueprints: %v", err)
	}
	if !strings.Contains(out, "Blueprints:") {
		t.Errorf("missing Blueprints section\ngot:\n%s", out)
	}
	if strings.Contains(out, "Extensions:") {
		t.Errorf("--blueprints should not print Extensions section\ngot:\n%s", out)
	}
}

// TestUnknownFlagErrors confirms cobra surfaces bad flags as errors rather than
// exiting; the parser, not the command body, owns this path.
func TestUnknownFlagErrors(t *testing.T) {
	_, err := execCmd(t, "list", "--definitely-not-a-flag")
	if err == nil {
		t.Fatal("expected an error for an unknown flag, got nil")
	}
}

// TestPlanArgsValidation exercises cobra's ExactArgs(1) guard on plan without
// reaching the command body (which would call os.Exit). Too few/many args must
// produce an error from Execute.
func TestPlanArgsValidation(t *testing.T) {
	for _, args := range [][]string{
		{"plan"},
		{"plan", "a", "b"},
	} {
		if _, err := execCmd(t, args...); err == nil {
			t.Errorf("plan %v: expected args validation error, got nil", args)
		}
	}
}

func TestServeArgsValidation(t *testing.T) {
	if _, err := execCmd(t, "serve"); err == nil {
		t.Error("serve with no args: expected args validation error, got nil")
	}
}

// TestHelpOutput ensures the root --help path renders without error and lists
// the registered subcommands.
func TestHelpOutput(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	rootCmd.SetArgs([]string{"--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("--help: %v", err)
	}
	help := buf.String()
	for _, want := range []string{"plan", "serve", "list", "version"} {
		if !strings.Contains(help, want) {
			t.Errorf("--help output missing %q\ngot:\n%s", want, help)
		}
	}
}
