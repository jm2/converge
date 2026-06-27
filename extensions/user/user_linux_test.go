//go:build linux

package user

import (
	"context"
	"errors"
	"os/exec"
	"os/user"
	"testing"
	"time"

	"github.com/TsekNet/converge/extensions"
	"github.com/TsekNet/converge/internal/testutil"
)

// withMockCmd installs a MockCmd as the newCommand seam for the duration of the
// test, restoring the original afterwards. The context passed by production code
// is ignored by the mock, which scripts results purely by command name.
func withMockCmd(t *testing.T, mock *testutil.MockCmd) {
	t.Helper()
	old := newCommand
	newCommand = func(_ context.Context, name string, args ...string) *exec.Cmd {
		return mock.Command(name, args...)
	}
	t.Cleanup(func() { newCommand = old })
}

func TestUser_modifyUser(t *testing.T) {
	tests := []struct {
		name        string
		opts        Opts
		usermodErr  error
		wantErr     bool
		wantChanged bool
		wantStatus  extensions.Status
		wantCalled  bool
		wantArgs    []string
	}{
		{
			name:        "no shell no groups is a no-op",
			opts:        Opts{},
			wantChanged: false,
			wantStatus:  extensions.StatusOK,
			wantCalled:  false,
		},
		{
			name:        "shell only",
			opts:        Opts{Shell: "/bin/zsh"},
			wantChanged: true,
			wantStatus:  extensions.StatusChanged,
			wantCalled:  true,
			wantArgs:    []string{"--shell", "/bin/zsh", "devuser"},
		},
		{
			name:        "groups appended",
			opts:        Opts{Groups: []string{"sudo", "docker"}},
			wantChanged: true,
			wantStatus:  extensions.StatusChanged,
			wantCalled:  true,
			wantArgs:    []string{"--append", "--groups", "sudo,docker", "devuser"},
		},
		{
			name:        "shell and groups",
			opts:        Opts{Shell: "/bin/bash", Groups: []string{"wheel"}},
			wantChanged: true,
			wantStatus:  extensions.StatusChanged,
			wantCalled:  true,
			wantArgs:    []string{"--shell", "/bin/bash", "--append", "--groups", "wheel", "devuser"},
		},
		{
			name:       "usermod failure surfaces error",
			opts:       Opts{Shell: "/bin/bash"},
			usermodErr: errors.New("boom"),
			wantErr:    true,
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockCmd()
			mock.SetOutput("usermod", "", tt.usermodErr)
			withMockCmd(t, mock)

			u := New("devuser", tt.opts)
			res, err := u.modifyUser(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("modifyUser() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("modifyUser() error = %v", err)
			}
			if res.Changed != tt.wantChanged {
				t.Errorf("Changed = %v, want %v", res.Changed, tt.wantChanged)
			}
			if res.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", res.Status, tt.wantStatus)
			}
			if mock.Called("usermod") != tt.wantCalled {
				t.Errorf("usermod called = %v, want %v (calls: %s)", mock.Called("usermod"), tt.wantCalled, mock)
			}
			if tt.wantCalled {
				calls := mock.CallsFor("usermod")
				if len(calls) != 1 {
					t.Fatalf("expected 1 usermod call, got %d", len(calls))
				}
				if got := calls[0].Args; !equalStrs(got, tt.wantArgs) {
					t.Errorf("usermod args = %v, want %v", got, tt.wantArgs)
				}
			}
		})
	}
}

func TestUser_createUser(t *testing.T) {
	tests := []struct {
		name       string
		opts       Opts
		useraddErr error
		wantErr    bool
		wantArgs   []string
	}{
		{
			name:     "minimal",
			opts:     Opts{},
			wantArgs: []string{"newuser"},
		},
		{
			name:     "system user with shell home and groups",
			opts:     Opts{System: true, Shell: "/bin/sh", Home: "/home/newuser", Groups: []string{"sudo", "docker"}},
			wantArgs: []string{"--system", "--shell", "/bin/sh", "--home-dir", "/home/newuser", "--create-home", "--groups", "sudo,docker", "newuser"},
		},
		{
			name:       "useradd failure surfaces error",
			opts:       Opts{},
			useraddErr: errors.New("boom"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockCmd()
			mock.SetOutput("useradd", "", tt.useraddErr)
			withMockCmd(t, mock)

			u := New("newuser", tt.opts)
			res, err := u.createUser(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("createUser() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("createUser() error = %v", err)
			}
			if !res.Changed || res.Status != extensions.StatusChanged {
				t.Errorf("res = %+v, want Changed=true Status=Changed", res)
			}
			calls := mock.CallsFor("useradd")
			if len(calls) != 1 {
				t.Fatalf("expected 1 useradd call, got %d", len(calls))
			}
			if got := calls[0].Args; !equalStrs(got, tt.wantArgs) {
				t.Errorf("useradd args = %v, want %v", got, tt.wantArgs)
			}
		})
	}
}

// TestUser_Apply_DispatchesByExistence verifies Apply routes to modifyUser for
// an existing user (the current user is guaranteed to exist) and to createUser
// for a nonexistent one, using the mocked command seam so no real account is
// touched.
func TestUser_Apply_DispatchesByExistence(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skip("cannot determine current user")
	}
	current := cur.Username

	t.Run("existing user -> usermod", func(t *testing.T) {
		mock := testutil.NewMockCmd()
		mock.SetOutput("usermod", "", nil)
		withMockCmd(t, mock)

		u := New(current, Opts{Shell: "/bin/bash"})
		if _, err := u.Apply(context.Background()); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		if !mock.Called("usermod") {
			t.Errorf("expected usermod for existing user, calls: %s", mock)
		}
		if mock.Called("useradd") {
			t.Errorf("did not expect useradd for existing user")
		}
	})

	t.Run("nonexistent user -> useradd", func(t *testing.T) {
		mock := testutil.NewMockCmd()
		mock.SetOutput("useradd", "", nil)
		withMockCmd(t, mock)

		u := New("converge-test-user-does-not-exist-xyz", Opts{Shell: "/bin/bash"})
		if _, err := u.Apply(context.Background()); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		if !mock.Called("useradd") {
			t.Errorf("expected useradd for nonexistent user, calls: %s", mock)
		}
		if mock.Called("usermod") {
			t.Errorf("did not expect usermod for nonexistent user")
		}
	})
}

// TestUser_Watch_ContextCancel verifies Watch establishes an inotify watch on
// /etc/passwd and returns cleanly when the context is cancelled. The event
// delivery path (writing to /etc/passwd) requires root and is not exercised
// here.
func TestUser_Watch_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan extensions.Event, 1)

	u := New("devuser", Opts{})
	done := make(chan error, 1)
	go func() { done <- u.Watch(ctx, events) }()

	// Give Watch a moment to register the watch, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Watch() returned error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch() did not return after context cancel")
	}
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
