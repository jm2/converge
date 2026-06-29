package main

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/TsekNet/converge/internal/exit"
	"github.com/TsekNet/converge/internal/output"
)

// makePrinter selects a concrete output.Printer from the global outputFormat
// flag. The default case depends on SupportsColor(); under `go test` stdout is
// not a TTY, so the default collapses to a SerialPrinter (the TerminalPrinter
// branch requires an interactive terminal and cannot be exercised here).
func TestMakePrinter(t *testing.T) {
	orig := outputFormat
	t.Cleanup(func() { outputFormat = orig })

	cases := []struct {
		format string
		want   output.Printer
	}{
		{"serial", (*output.SerialPrinter)(nil)},
		{"json", (*output.JSONPrinter)(nil)},
		// Unknown formats and the explicit "terminal" default both fall through
		// to the default case, which yields a SerialPrinter without a TTY.
		{"terminal", (*output.SerialPrinter)(nil)},
		{"bogus", (*output.SerialPrinter)(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			outputFormat = tc.format
			got := makePrinter()
			if got == nil {
				t.Fatal("makePrinter returned nil")
			}
			switch tc.want.(type) {
			case *output.SerialPrinter:
				if _, ok := got.(*output.SerialPrinter); !ok {
					t.Fatalf("format %q: got %T, want *output.SerialPrinter", tc.format, got)
				}
			case *output.JSONPrinter:
				if _, ok := got.(*output.JSONPrinter); !ok {
					t.Fatalf("format %q: got %T, want *output.JSONPrinter", tc.format, got)
				}
			}
		})
	}
}

// TestMakePrinterJSONNoColor confirms the default branch still returns a
// SerialPrinter even with NO_COLOR set (SupportsColor returns false), and that
// the json/serial branches are unaffected by terminal capabilities.
func TestMakePrinterDefaultNoColor(t *testing.T) {
	orig := outputFormat
	t.Cleanup(func() { outputFormat = orig })
	t.Setenv("NO_COLOR", "1")

	outputFormat = "terminal"
	if _, ok := makePrinter().(*output.SerialPrinter); !ok {
		t.Fatalf("with NO_COLOR set, default makePrinter should yield *output.SerialPrinter, got %T", makePrinter())
	}
}

func TestSimplifyExit(t *testing.T) {
	origDetailed := detailedExitCodes
	t.Cleanup(func() { detailedExitCodes = origDetailed })

	cases := []struct {
		name     string
		detailed bool
		code     int
		want     int
	}{
		// Collapsed (default) mapping: per docs/cli.md, OK/Changed/Pending are
		// "success" and collapse to 0; everything else collapses to 1.
		{"collapsed OK", false, exit.OK, 0},
		{"collapsed Changed", false, exit.Changed, 0},
		{"collapsed Pending", false, exit.Pending, 0},
		{"collapsed Error", false, exit.Error, 1},
		{"collapsed PartialFail", false, exit.PartialFail, 1},
		{"collapsed AllFailed", false, exit.AllFailed, 1},
		{"collapsed NotRoot", false, exit.NotRoot, 1},
		{"collapsed NotFound", false, exit.NotFound, 1},
		// Detailed mapping passes the code through unchanged.
		{"detailed OK", true, exit.OK, exit.OK},
		{"detailed Changed", true, exit.Changed, exit.Changed},
		{"detailed PartialFail", true, exit.PartialFail, exit.PartialFail},
		{"detailed AllFailed", true, exit.AllFailed, exit.AllFailed},
		{"detailed Pending", true, exit.Pending, exit.Pending},
		{"detailed NotRoot", true, exit.NotRoot, exit.NotRoot},
		{"detailed NotFound", true, exit.NotFound, exit.NotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detailedExitCodes = tc.detailed
			if got := simplifyExit(tc.code); got != tc.want {
				t.Errorf("simplifyExit(%d) detailed=%v = %d, want %d", tc.code, tc.detailed, got, tc.want)
			}
		})
	}
}

// exitWithCode and exitWithError both call os.Exit, so they are exercised in a
// re-executed child process. The child branch is selected by env vars and the
// parent asserts the observed process exit status, which is the value
// simplifyExit produces for the given code/flag combination.
func TestExitWithCodeAndError(t *testing.T) {
	if code := os.Getenv("CONVERGE_TEST_EXIT_CODE"); code != "" {
		c, _ := strconv.Atoi(code)
		detailedExitCodes = os.Getenv("CONVERGE_TEST_EXIT_DETAILED") == "1"
		if os.Getenv("CONVERGE_TEST_EXIT_FN") == "error" {
			exitWithError(c, errTestExit)
		}
		exitWithCode(c)
		return // unreachable; os.Exit fired
	}

	cases := []struct {
		name     string
		fn       string
		code     int
		detailed bool
		want     int
	}{
		{"code Changed collapsed", "code", exit.Changed, false, 0},
		{"code Error collapsed", "code", exit.Error, false, 1},
		{"code AllFailed detailed", "code", exit.AllFailed, true, exit.AllFailed},
		{"error NotFound collapsed", "error", exit.NotFound, false, 1},
		{"error Pending collapsed", "error", exit.Pending, false, 0},
		{"error NotRoot detailed", "error", exit.NotRoot, true, exit.NotRoot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestExitWithCodeAndError")
			cmd.Env = append(os.Environ(),
				"CONVERGE_TEST_EXIT_CODE="+strconv.Itoa(tc.code),
				"CONVERGE_TEST_EXIT_FN="+tc.fn,
			)
			if tc.detailed {
				cmd.Env = append(cmd.Env, "CONVERGE_TEST_EXIT_DETAILED=1")
			}
			out, err := cmd.CombinedOutput()
			got := 0
			if err != nil {
				ee, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("unexpected error type %T: %v", err, err)
				}
				got = ee.ExitCode()
			}
			if got != tc.want {
				t.Fatalf("child exit = %d, want %d (output: %q)", got, tc.want, out)
			}
			// exitWithError must emit the message on stderr (captured via
			// CombinedOutput) before exiting.
			if tc.fn == "error" && len(out) == 0 {
				t.Errorf("exitWithError produced no output; expected an Error: line")
			}
		})
	}
}

var errTestExit = errTest("boom")

type errTest string

func (e errTest) Error() string { return string(e) }

// TestRootPersistentFlags pins the wiring of the root command's persistent
// flags: names, shorthands, and default values that the rest of the CLI relies
// on. A regression here (renamed flag, wrong default) would silently change
// behavior for every subcommand.
func TestRootPersistentFlags(t *testing.T) {
	pf := rootCmd.PersistentFlags()

	if f := pf.Lookup("out"); f == nil {
		t.Error(`missing persistent flag "out"`)
	} else if f.DefValue != "terminal" {
		t.Errorf(`out default = %q, want "terminal"`, f.DefValue)
	}

	if f := pf.Lookup("verbose"); f == nil {
		t.Error(`missing persistent flag "verbose"`)
	} else if f.Shorthand != "v" {
		t.Errorf(`verbose shorthand = %q, want "v"`, f.Shorthand)
	}

	if f := pf.Lookup("resource-timeout"); f == nil {
		t.Error(`missing persistent flag "resource-timeout"`)
	} else if f.DefValue != (5 * time.Minute).String() {
		t.Errorf(`resource-timeout default = %q, want %q`, f.DefValue, (5 * time.Minute).String())
	}

	if f := pf.Lookup("parallel"); f == nil {
		t.Error(`missing persistent flag "parallel"`)
	} else if f.DefValue != "1" {
		t.Errorf(`parallel default = %q, want "1"`, f.DefValue)
	}

	if f := pf.Lookup("detailed-exit-codes"); f == nil {
		t.Error(`missing persistent flag "detailed-exit-codes"`)
	} else if f.DefValue != "false" {
		t.Errorf(`detailed-exit-codes default = %q, want "false"`, f.DefValue)
	}
}

// TestRootSubcommandsRegistered ensures every expected subcommand is wired onto
// rootCmd via the various init() functions in this package.
func TestRootSubcommandsRegistered(t *testing.T) {
	want := []string{"list", "version", "plan", "serve"}
	have := map[string]bool{}
	for _, c := range rootCmd.Commands() {
		have[c.Name()] = true
	}
	for _, name := range want {
		if !have[name] {
			t.Errorf("subcommand %q not registered on rootCmd", name)
		}
	}
}
