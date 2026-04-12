package template

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/TsekNet/converge/internal/testutil"
)

func TestTemplate_ID(t *testing.T) {
	tmpl := New("/etc/nginx/nginx.conf", Opts{})
	if got := tmpl.ID(); got != "template:/etc/nginx/nginx.conf" {
		t.Errorf("ID() = %q, want %q", got, "template:/etc/nginx/nginx.conf")
	}
}

func TestTemplate_String(t *testing.T) {
	tmpl := New("/etc/nginx/nginx.conf", Opts{})
	if got := tmpl.String(); got != "Template /etc/nginx/nginx.conf" {
		t.Errorf("String() = %q, want %q", got, "Template /etc/nginx/nginx.conf")
	}
}

func TestTemplate_IsCritical(t *testing.T) {
	tmpl := New("/tmp/test", Opts{})
	if tmpl.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	tmpl2 := New("/tmp/test", Opts{Critical: true})
	if !tmpl2.IsCritical() {
		t.Error("IsCritical() should be true when set via Opts")
	}
}

func TestTemplate_Check(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string)
		tmpl        func(dir string) *Template
		wantSync    bool
		wantChanges int
	}{
		{
			"file does not exist",
			func(t *testing.T, dir string) {},
			func(dir string) *Template {
				return New(filepath.Join(dir, "new.conf"), Opts{Source: "server {{ .Host }}", Vars: map[string]string{"Host": "localhost"}, Mode: 0644})
			},
			false, 2,
		},
		{
			"file matches rendered output",
			func(t *testing.T, dir string) {
				t.Helper()
				p := filepath.Join(dir, "ok.conf")
				os.WriteFile(p, []byte("server localhost"), 0644)
				os.Chmod(p, 0644)
			},
			func(dir string) *Template {
				return New(filepath.Join(dir, "ok.conf"), Opts{Source: "server {{ .Host }}", Vars: map[string]string{"Host": "localhost"}, Mode: 0644})
			},
			true, 0,
		},
		{
			"file content differs",
			func(t *testing.T, dir string) {
				t.Helper()
				p := filepath.Join(dir, "drift.conf")
				os.WriteFile(p, []byte("server old-host"), 0644)
				os.Chmod(p, 0644)
			},
			func(dir string) *Template {
				return New(filepath.Join(dir, "drift.conf"), Opts{Source: "server {{ .Host }}", Vars: map[string]string{"Host": "new-host"}, Mode: 0644})
			},
			false, 1,
		},
		{
			"mode differs",
			func(t *testing.T, dir string) {
				t.Helper()
				p := filepath.Join(dir, "mode.conf")
				os.WriteFile(p, []byte("server localhost"), 0755)
				os.Chmod(p, 0755)
			},
			func(dir string) *Template {
				return New(filepath.Join(dir, "mode.conf"), Opts{Source: "server {{ .Host }}", Vars: map[string]string{"Host": "localhost"}, Mode: 0644})
			},
			false, 1,
		},
		{
			"nil vars with no placeholders",
			func(t *testing.T, dir string) {
				t.Helper()
				p := filepath.Join(dir, "static.conf")
				os.WriteFile(p, []byte("static content"), 0644)
				os.Chmod(p, 0644)
			},
			func(dir string) *Template {
				return New(filepath.Join(dir, "static.conf"), Opts{Source: "static content", Mode: 0644})
			},
			true, 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			tmpl := tt.tmpl(dir)

			state, err := tmpl.Check(ctx)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if state.InSync != tt.wantSync {
				t.Errorf("InSync = %v, want %v", state.InSync, tt.wantSync)
			}
			if len(state.Changes) != tt.wantChanges {
				t.Errorf("len(Changes) = %d, want %d: %+v", len(state.Changes), tt.wantChanges, state.Changes)
			}
		})
	}
}

func TestTemplate_Apply(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		setup  func(t *testing.T, dir string)
		tmpl   func(dir string) *Template
		verify func(t *testing.T, dir string)
	}{
		{
			"create new file from template",
			func(t *testing.T, dir string) {},
			func(dir string) *Template {
				return New(filepath.Join(dir, "new.conf"), Opts{
					Source: "listen {{ .Port }}\nhost {{ .Host }}\n",
					Vars:   map[string]string{"Port": "8080", "Host": "0.0.0.0"},
					Mode:   0644,
				})
			},
			func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, "new.conf"))
				if err != nil {
					t.Fatalf("read: %v", err)
				}
				want := "listen 8080\nhost 0.0.0.0\n"
				if string(data) != want {
					t.Errorf("content = %q, want %q", data, want)
				}
			},
		},
		{
			"overwrite existing file",
			func(t *testing.T, dir string) {
				t.Helper()
				os.WriteFile(filepath.Join(dir, "exist.conf"), []byte("old"), 0644)
			},
			func(dir string) *Template {
				return New(filepath.Join(dir, "exist.conf"), Opts{Source: "new={{ .Val }}", Vars: map[string]string{"Val": "42"}, Mode: 0644})
			},
			func(t *testing.T, dir string) {
				t.Helper()
				data, _ := os.ReadFile(filepath.Join(dir, "exist.conf"))
				if string(data) != "new=42" {
					t.Errorf("content = %q, want %q", data, "new=42")
				}
			},
		},
		{
			"create nested directories",
			func(t *testing.T, dir string) {},
			func(dir string) *Template {
				return New(filepath.Join(dir, "sub", "deep", "file.conf"), Opts{Source: "nested", Mode: 0644})
			},
			func(t *testing.T, dir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(dir, "sub", "deep", "file.conf"))
				if err != nil {
					t.Fatalf("read: %v", err)
				}
				if string(data) != "nested" {
					t.Errorf("content = %q", data)
				}
			},
		},
		{
			"set file mode",
			func(t *testing.T, dir string) {},
			func(dir string) *Template {
				return New(filepath.Join(dir, "secret.conf"), Opts{Source: "key={{ .Key }}", Vars: map[string]string{"Key": "abc"}, Mode: 0600})
			},
			func(t *testing.T, dir string) {
				t.Helper()
				info, _ := os.Stat(filepath.Join(dir, "secret.conf"))
				if info.Mode().Perm() != 0600 {
					t.Errorf("mode = %04o, want 0600", info.Mode().Perm())
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			tmpl := tt.tmpl(dir)

			result, err := tmpl.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if !result.Changed {
				t.Error("Changed should be true")
			}
			tt.verify(t, dir)
		})
	}
}

func TestTemplate_CheckThenApplyIdempotent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "idem.conf")

	tmpl := New(path, Opts{Source: "value={{ .X }}", Vars: map[string]string{"X": "1"}, Mode: 0644})

	state, _ := tmpl.Check(ctx)
	if state.InSync {
		t.Fatal("should not be in sync before Apply")
	}

	tmpl.Apply(ctx)

	tmpl2 := New(path, Opts{Source: "value={{ .X }}", Vars: map[string]string{"X": "1"}, Mode: 0644})
	state, _ = tmpl2.Check(ctx)
	if !state.InSync {
		t.Errorf("should be in sync after Apply, changes: %+v", state.Changes)
	}
}

func TestTemplate_Render_InvalidTemplate(t *testing.T) {
	ctx := context.Background()
	tmpl := New("/tmp/bad", Opts{Source: "{{ .Unclosed"})

	_, err := tmpl.Check(ctx)
	if err == nil {
		t.Error("Check() should fail with invalid template syntax")
	}
}

func TestTemplate_Render_MissingVar(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	tmpl := New(filepath.Join(dir, "missing.conf"), Opts{Source: "value={{ .Missing }}", Vars: map[string]string{}, Mode: 0644})

	_, err := tmpl.Check(ctx)
	if err == nil {
		t.Error("Check() should fail when template references missing variable")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is a ..."},
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

func TestTemplate_MapFS_CheckAndApply(t *testing.T) {
	mfs := testutil.NewMapFS()

	tmpl := New("/etc/nginx.conf", Opts{
		Source: "server {{ .Host }}",
		Vars:   map[string]string{"Host": "localhost"},
		Mode:   0644,
		FS:     mfs,
	})
	testutil.AssertConverges(t, tmpl)

	data, ok := mfs.Get("/etc/nginx.conf")
	if !ok {
		t.Fatal("file should exist in MapFS")
	}
	if string(data) != "server localhost" {
		t.Errorf("content = %q, want %q", data, "server localhost")
	}
}
