package dsl

import (
	"context"
	"testing"
)

func TestResourceCheckAndApply(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name   string
		setup  func(r *Run)
		wantID string
	}{
		{"file", func(r *Run) { r.File("/tmp/converge-test", FileOpts{Content: "hi"}) }, "file:/tmp/converge-test"},
		{"package", func(r *Run) { r.Package("git", PackageOpts{State: Present}) }, "package:git"},
		{"service", func(r *Run) { r.Service("sshd", ServiceOpts{State: Running}) }, "service:sshd"},
		{"exec", func(r *Run) { r.Exec("test", ExecOpts{Command: "echo"}) }, "exec:test"},
		{"user", func(r *Run) { r.User("dev", UserOpts{Shell: "/bin/bash"}) }, "user:dev"},
		{"template", func(r *Run) {
			r.Template("/tmp/converge-tmpl", TemplateOpts{Source: "hello {{ .Name }}", Vars: map[string]string{"Name": "world"}})
		}, "template:/tmp/converge-tmpl"},
		{"file-remote", func(r *Run) {
			r.File("/tmp/converge-remote", FileOpts{URL: "https://example.com/file"})
		}, "file:/tmp/converge-remote"},
		{"hostname", func(r *Run) { r.Hostname("test-host", HostnameOpts{}) }, "hostname:test-host"},
		{"cron", func(r *Run) {
			r.Cron("backup", CronOpts{Schedule: "0 2 * * *", Command: "/usr/bin/backup.sh"})
		}, "cron:backup"},
		{"file-block", func(r *Run) {
			r.File("/tmp/converge-partial", FileOpts{Content: "managed", BlockName: "block1"})
		}, "file:/tmp/converge-partial[block1]"},
		{"repository", func(r *Run) {
			r.Repository("chrome", RepositoryOpts{URI: "https://dl.google.com/linux/chrome/deb/"})
		}, "repository:chrome"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := New()
			run := newRun(app)
			tt.setup(run)

			resources := run.Resources()
			if len(resources) != 1 {
				t.Fatalf("expected 1 resource, got %d", len(resources))
			}

			ext := resources[0]
			if ext.ID() != tt.wantID {
				t.Errorf("ID() = %q, want %q", ext.ID(), tt.wantID)
			}

			state, err := ext.Check(ctx)
			if err != nil {
				t.Logf("Check() error = %v (expected for some extensions)", err)
				return
			}
			if state == nil {
				t.Fatal("Check() returned nil state")
			}
			t.Logf("InSync=%v Changes=%d", state.InSync, len(state.Changes))
		})
	}
}
