package user

import (
	"context"
	"os/user"
	"reflect"
	"testing"
)

func TestUser_IDAndString(t *testing.T) {
	tests := []struct {
		name    string
		wantID  string
		wantStr string
	}{
		{"devuser", "user:devuser", "User devuser"},
		{"admin", "user:admin", "User admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := New(tt.name, Opts{})
			if u.ID() != tt.wantID {
				t.Errorf("ID() = %q, want %q", u.ID(), tt.wantID)
			}
			if u.String() != tt.wantStr {
				t.Errorf("String() = %q, want %q", u.String(), tt.wantStr)
			}
		})
	}
}

func TestUser_Check_ExistingUser(t *testing.T) {
	ctx := context.Background()

	current, err := user.Current()
	if err != nil {
		t.Skip("cannot determine current user")
	}

	u := New(current.Username, Opts{})
	state, err := u.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !state.InSync {
		t.Logf("current user not fully in sync: %+v (expected for no shell specified)", state.Changes)
	}
}

func TestUser_Check_NonexistentUser(t *testing.T) {
	ctx := context.Background()

	u := New("converge-test-user-does-not-exist-xyz", Opts{Groups: []string{"sudo"}, Shell: "/bin/bash"})
	state, err := u.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("nonexistent user should not be in sync")
	}
	if len(state.Changes) < 1 {
		t.Error("should have at least one change (create user)")
	}
}

func TestUser_Check_WithShell(t *testing.T) {
	ctx := context.Background()

	current, err := user.Current()
	if err != nil {
		t.Skip("cannot determine current user")
	}

	tests := []struct {
		name  string
		shell string
	}{
		{"matching shell", "/bin/bash"},
		{"different shell", "/usr/bin/zsh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := New(current.Username, Opts{Shell: tt.shell})
			state, err := u.Check(ctx)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			t.Logf("InSync=%v Changes=%+v", state.InSync, state.Changes)
		})
	}
}

func TestUser_IsCritical(t *testing.T) {
	u := New("devuser", Opts{})
	if u.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	u2 := New("devuser", Opts{Critical: true})
	if !u2.IsCritical() {
		t.Error("IsCritical() should be true when set")
	}
}

func TestUser_New(t *testing.T) {
	u := New("admin", Opts{Groups: []string{"sudo", "docker"}, Shell: "/bin/zsh"})
	if u.Name != "admin" {
		t.Errorf("Name = %q, want %q", u.Name, "admin")
	}
	if len(u.Groups) != 2 {
		t.Errorf("Groups len = %d, want 2", len(u.Groups))
	}
	if u.Shell != "/bin/zsh" {
		t.Errorf("Shell = %q, want %q", u.Shell, "/bin/zsh")
	}
}

func TestMissingGroups(t *testing.T) {
	tests := []struct {
		name    string
		desired []string
		current []string
		want    []string
	}{
		{"no desired", nil, []string{"a", "b"}, nil},
		{"all present", []string{"a"}, []string{"a", "b"}, nil},
		{"one missing", []string{"a", "c"}, []string{"a", "b"}, []string{"c"}},
		{"all missing", []string{"x", "y"}, []string{"a"}, []string{"x", "y"}},
		{"empty current", []string{"a"}, nil, []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := missingGroups(tt.desired, tt.current)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("missingGroups(%v, %v) = %v, want %v", tt.desired, tt.current, got, tt.want)
			}
		})
	}
}

func TestUser_Check_GroupDrift(t *testing.T) {
	ctx := context.Background()

	current, err := user.Current()
	if err != nil {
		t.Skip("cannot determine current user")
	}

	// A group the current user is almost certainly not a member of, so Check
	// must report group drift.
	u := New(current.Username, Opts{Groups: []string{"converge-nonexistent-group-xyz"}})
	state, err := u.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("expected drift for non-member group membership")
	}
	found := false
	for _, c := range state.Changes {
		if c.Property == "groups" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a 'groups' change reporting drift, got %+v", state.Changes)
	}
}

func TestUser_Check_GroupNoDrift(t *testing.T) {
	ctx := context.Background()

	current, err := user.Current()
	if err != nil {
		t.Skip("cannot determine current user")
	}

	groups, err := userGroups(current)
	if err != nil || len(groups) == 0 {
		t.Skip("current user has no readable groups")
	}

	// Requesting a group the user already belongs to must not report drift.
	u := New(current.Username, Opts{Groups: []string{groups[0]}})
	state, err := u.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	for _, c := range state.Changes {
		if c.Property == "groups" {
			t.Errorf("unexpected group drift for member group %q: %+v", groups[0], state.Changes)
		}
	}
}

func TestUser_Apply_NonexistentUser(t *testing.T) {
	ctx := context.Background()
	u := New("converge-test-user-does-not-exist-xyz", Opts{Groups: []string{"sudo"}, Shell: "/bin/bash"})
	_, err := u.Apply(ctx)
	if err == nil {
		t.Log("Apply may succeed if running as root, or fail otherwise")
	}
}
