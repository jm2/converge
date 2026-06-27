package pkg

import (
	"context"
	"strings"
	"testing"

	"github.com/TsekNet/converge/internal/testutil"
)

func TestRepository_Validate_RejectsInjection(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		field string
		repo  *Repository
	}{
		{"newline in Name", "Name", &Repository{Name: "a\nb", URI: "u", ManagerName: "apt"}},
		{"carriage return in URI", "URI", &Repository{Name: "a", URI: "u\rx", ManagerName: "apt"}},
		{"null in Components", "Components", &Repository{Name: "a", URI: "u", Components: "m\x00n", ManagerName: "apt"}},
		{"newline in GPGKey", "GPGKey", &Repository{Name: "a", URI: "u", GPGKey: "k\ny", ManagerName: "apt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.repo.Check(ctx); err == nil {
				t.Errorf("Check() should reject invalid %s", tt.field)
			}
			if _, err := tt.repo.Apply(ctx); err == nil {
				t.Errorf("Apply() should reject invalid %s", tt.field)
			}
		})
	}
}

func TestRepository_RepoContent_UnsupportedManager(t *testing.T) {
	r := &Repository{Name: "x", URI: "u", ManagerName: "brew"}
	if got := r.repoContent(); got != "" {
		t.Errorf("repoContent() = %q, want empty for unsupported manager", got)
	}
}

func TestRepository_Check_FileExists(t *testing.T) {
	ctx := context.Background()

	t.Run("present and matching is in sync", func(t *testing.T) {
		mfs := testutil.NewMapFS()
		r := &Repository{Name: "chrome", URI: "https://example.com/", Distribution: "stable", Components: "main", ManagerName: "apt", State: "present", FS: mfs}
		mfs.Set(r.repoFilePath(), []byte(r.repoContent()), 0644)
		testutil.AssertInSync(t, r)
	})

	t.Run("present but content differs is drifted", func(t *testing.T) {
		mfs := testutil.NewMapFS()
		r := &Repository{Name: "chrome", URI: "https://example.com/", Distribution: "stable", Components: "main", ManagerName: "apt", State: "present", FS: mfs}
		mfs.Set(r.repoFilePath(), []byte("deb https://old.example.com/ old main\n"), 0644)
		state, err := r.Check(ctx)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if state.InSync {
			t.Error("should be drifted when content differs")
		}
		if len(state.Changes) != 1 || state.Changes[0].Action != "modify" {
			t.Errorf("want one modify change, got %+v", state.Changes)
		}
	})

	t.Run("absent but file present is drifted", func(t *testing.T) {
		mfs := testutil.NewMapFS()
		r := &Repository{Name: "chrome", URI: "https://example.com/", Distribution: "stable", Components: "main", ManagerName: "apt", State: "absent", FS: mfs}
		mfs.Set(r.repoFilePath(), []byte("deb https://example.com/ stable main\n"), 0644)
		state, err := r.Check(ctx)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if state.InSync {
			t.Error("should be drifted when file exists but state is absent")
		}
		if len(state.Changes) != 1 || state.Changes[0].Action != "remove" {
			t.Errorf("want one remove change, got %+v", state.Changes)
		}
	})
}

func TestRepository_Apply_RemovesExistingFile(t *testing.T) {
	mfs := testutil.NewMapFS()
	r := &Repository{Name: "chrome", URI: "https://example.com/", ManagerName: "apt", State: "absent", FS: mfs}
	path := r.repoFilePath()
	mfs.Set(path, []byte("deb https://example.com/ stable main\n"), 0644)

	res, err := r.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !res.Changed || res.Message != "removed" {
		t.Errorf("result = %+v, want changed/removed", res)
	}
	if mfs.Has(path) {
		t.Error("repo file should be removed")
	}
}

func TestRepository_Apply_AbsentNoFile(t *testing.T) {
	mfs := testutil.NewMapFS()
	r := &Repository{Name: "chrome", URI: "https://example.com/", ManagerName: "apt", State: "absent", FS: mfs}

	res, err := r.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !res.Changed || res.Message != "removed" {
		t.Errorf("result = %+v, want changed/removed even when file absent", res)
	}
}

func TestRepository_Check_DnfModify(t *testing.T) {
	mfs := testutil.NewMapFS()
	r := &Repository{Name: "epel", URI: "https://epel.example.com/", ManagerName: "dnf", Enabled: true, State: "present", FS: mfs}
	mfs.Set(r.repoFilePath(), []byte("[epel]\nname=epel\nbaseurl=https://stale/\nenabled=0\ngpgcheck=1\n"), 0644)

	state, err := r.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("dnf repo with stale content should be drifted")
	}
	if len(state.Changes) != 1 || !strings.Contains(state.Changes[0].To, "baseurl") {
		t.Errorf("want modify change referencing new content, got %+v", state.Changes)
	}
}
