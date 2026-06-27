package shell

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name         string
		shell        string
		customParams []string
		wantBinary   string
		wantParams   []string
		wantResolved string
	}{
		{
			name:         "powershell defaults",
			shell:        PowerShell,
			wantBinary:   defaultPowerShellPath,
			wantParams:   defaultPSParams,
			wantResolved: PowerShell,
		},
		{
			name:         "powershell custom params",
			shell:        PowerShell,
			customParams: []string{"-Foo"},
			wantBinary:   defaultPowerShellPath,
			wantParams:   []string{"-Foo"},
			wantResolved: PowerShell,
		},
		{
			name:         "pwsh defaults",
			shell:        Pwsh,
			wantBinary:   "pwsh",
			wantParams:   defaultPSParams,
			wantResolved: Pwsh,
		},
		{
			name:         "pwsh custom params",
			shell:        Pwsh,
			customParams: []string{"-NoLogo"},
			wantBinary:   "pwsh",
			wantParams:   []string{"-NoLogo"},
			wantResolved: Pwsh,
		},
		{
			name:         "cmd has no params",
			shell:        Cmd,
			wantBinary:   `C:\Windows\System32\cmd.exe`,
			wantParams:   nil,
			wantResolved: Cmd,
		},
		{
			name:         "bash",
			shell:        Bash,
			wantBinary:   "/bin/bash",
			wantParams:   nil,
			wantResolved: Bash,
		},
		{
			name:         "sh",
			shell:        Sh,
			wantBinary:   "/bin/sh",
			wantParams:   nil,
			wantResolved: Sh,
		},
		{
			name:         "custom path passthrough",
			shell:        "/opt/custom/myshell",
			wantBinary:   "/opt/custom/myshell",
			wantParams:   nil,
			wantResolved: "/opt/custom/myshell",
		},
		{
			name:         "custom path with custom params",
			shell:        "/opt/custom/myshell",
			customParams: []string{"-x"},
			wantBinary:   "/opt/custom/myshell",
			wantParams:   []string{"-x"},
			wantResolved: "/opt/custom/myshell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binary, params, resolved := Resolve(tt.shell, tt.customParams)
			if binary != tt.wantBinary {
				t.Errorf("binary = %q, want %q", binary, tt.wantBinary)
			}
			if !reflect.DeepEqual(params, tt.wantParams) {
				t.Errorf("params = %v, want %v", params, tt.wantParams)
			}
			if resolved != tt.wantResolved {
				t.Errorf("resolved = %q, want %q", resolved, tt.wantResolved)
			}
		})
	}
}

func TestResolveAuto(t *testing.T) {
	binary, params, resolved := Resolve(Auto, nil)

	if runtime.GOOS == "windows" {
		if resolved != PowerShell {
			t.Errorf("resolved = %q, want %q", resolved, PowerShell)
		}
		if binary != defaultPowerShellPath {
			t.Errorf("binary = %q, want %q", binary, defaultPowerShellPath)
		}
		if !reflect.DeepEqual(params, defaultPSParams) {
			t.Errorf("params = %v, want %v", params, defaultPSParams)
		}
		return
	}

	if resolved != Bash {
		t.Errorf("resolved = %q, want %q", resolved, Bash)
	}
	if binary != "/bin/bash" {
		t.Errorf("binary = %q, want %q", binary, "/bin/bash")
	}
	if params != nil {
		t.Errorf("params = %v, want nil", params)
	}
}

func TestCommand(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		shell    string
		command  string
		params   []string
		wantArgs []string // args after the binary (cmd.Args[1:])
	}{
		{
			name:     "bash uses -c",
			shell:    Bash,
			command:  "echo hi",
			wantArgs: []string{"-c", "echo hi"},
		},
		{
			name:     "sh uses -c",
			shell:    Sh,
			command:  "echo hi",
			wantArgs: []string{"-c", "echo hi"},
		},
		{
			name:     "cmd uses /C",
			shell:    Cmd,
			command:  "echo hi",
			wantArgs: []string{"/C", "echo hi"},
		},
		{
			name:    "powershell wraps with prefix/suffix and -Command",
			shell:   PowerShell,
			command: "Get-Process",
			wantArgs: append(append([]string{}, defaultPSParams...),
				"-Command", psPrefix+"Get-Process"+psSuffix),
		},
		{
			name:    "pwsh wraps with prefix/suffix and -Command",
			shell:   Pwsh,
			command: "Get-Process",
			wantArgs: append(append([]string{}, defaultPSParams...),
				"-Command", psPrefix+"Get-Process"+psSuffix),
		},
		{
			name:     "bash with custom params",
			shell:    Bash,
			command:  "echo hi",
			params:   []string{"-e"},
			wantArgs: []string{"-e", "-c", "echo hi"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Command(ctx, tt.shell, tt.command, tt.params)
			gotArgs := cmd.Args[1:]
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("args = %v, want %v", gotArgs, tt.wantArgs)
			}
			wantBinary, _, _ := Resolve(tt.shell, tt.params)
			// On Windows, exec.Command resolves a bare name like "pwsh" to its
			// full path with a ".exe" suffix (e.g. ...\pwsh.exe). Windows/macOS
			// filesystems are also case-insensitive, so compare case-folded and
			// allow the ".exe" suffix.
			gotPath := strings.ToLower(cmd.Path)
			wantPath := strings.ToLower(wantBinary)
			if gotPath != wantPath &&
				!strings.HasSuffix(gotPath, wantPath) &&
				!strings.HasSuffix(gotPath, wantPath+".exe") {
				t.Errorf("path = %q, want %q", cmd.Path, wantBinary)
			}
		})
	}
}

func TestRun(t *testing.T) {
	if _, err := exec.LookPath("/bin/bash"); err != nil {
		t.Skip("/bin/bash not available")
	}
	ctx := context.Background()

	t.Run("trims stdout", func(t *testing.T) {
		out, err := Run(ctx, Bash, "echo hello", nil)
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if out != "hello" {
			t.Errorf("out = %q, want %q", out, "hello")
		}
	})

	t.Run("empty output", func(t *testing.T) {
		out, err := Run(ctx, Bash, "true", nil)
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if out != "" {
			t.Errorf("out = %q, want empty", out)
		}
	})

	t.Run("nonzero exit returns error", func(t *testing.T) {
		_, err := Run(ctx, Bash, "exit 3", nil)
		if err == nil {
			t.Fatal("expected error for nonzero exit, got nil")
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{name: "short unchanged", in: "abc", maxLen: 10, want: "abc"},
		{name: "exact length", in: "abcde", maxLen: 5, want: "abcde"},
		{name: "too long truncated", in: "abcdefgh", maxLen: 3, want: "abc..."},
		{name: "trailing newline stripped", in: "abc\n", maxLen: 10, want: "abc"},
		{name: "trailing crlf stripped", in: "abc\r\n", maxLen: 10, want: "abc"},
		{name: "multiple trailing newlines stripped", in: "abc\n\n\n", maxLen: 10, want: "abc"},
		{name: "strip then truncate", in: "abcdefgh\n", maxLen: 3, want: "abc..."},
		{name: "empty", in: "", maxLen: 5, want: ""},
		{name: "only newlines", in: "\n\r", maxLen: 5, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Truncate(tt.in, tt.maxLen); got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestIsPowerShell(t *testing.T) {
	tests := []struct {
		shell string
		want  bool
	}{
		{shell: PowerShell, want: true},
		{shell: Pwsh, want: true},
		{shell: Bash, want: false},
		{shell: Sh, want: false},
		{shell: Cmd, want: false},
		{shell: "/opt/custom", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			if got := IsPowerShell(tt.shell); got != tt.want {
				t.Errorf("IsPowerShell(%q) = %v, want %v", tt.shell, got, tt.want)
			}
		})
	}

	t.Run("auto", func(t *testing.T) {
		want := runtime.GOOS == "windows"
		if got := IsPowerShell(Auto); got != want {
			t.Errorf("IsPowerShell(auto) = %v, want %v", got, want)
		}
	})
}

func TestFormatOutput(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected string
		err      error
		want     string
	}{
		{
			name: "error with empty actual",
			err:  errors.New("boom"),
			want: "(error: boom)",
		},
		{
			name:   "actual present with error returns actual",
			actual: "some output",
			err:    errors.New("boom"),
			want:   "some output",
		},
		{
			name:   "actual present no error",
			actual: "ok",
			want:   "ok",
		},
		{
			name: "empty actual no error",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutput(tt.actual, tt.expected, tt.err); got != tt.want {
				t.Errorf("FormatOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}
