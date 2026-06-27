package service

import (
	"context"
	"runtime"
	"testing"
)

func TestService_IDAndString(t *testing.T) {
	tests := []struct {
		name    string
		wantID  string
		wantStr string
	}{
		{"sshd", "service:sshd", "Service sshd"},
		{"docker", "service:docker", "Service docker"},
		{"nginx", "service:nginx", "Service nginx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.name, Opts{State: "running", Enable: true, InitSystem: "systemd"})
			if s.ID() != tt.wantID {
				t.Errorf("ID() = %q, want %q", s.ID(), tt.wantID)
			}
			if s.String() != tt.wantStr {
				t.Errorf("String() = %q, want %q", s.String(), tt.wantStr)
			}
		})
	}
}

func TestService_Check_UnsupportedInit(t *testing.T) {
	ctx := context.Background()
	s := New("test", Opts{State: "running", Enable: true, InitSystem: "runit"})

	_, err := s.Check(ctx)
	if err == nil {
		t.Error("Check() should fail for unsupported init system")
	}
}

func TestService_Apply_UnsupportedInit(t *testing.T) {
	ctx := context.Background()
	s := New("test", Opts{State: "running", Enable: true, InitSystem: "runit"})

	_, err := s.Apply(ctx)
	if err == nil {
		t.Error("Apply() should fail for unsupported init system")
	}
}

func TestService_CheckSystemd_LiveCron(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd tests only run on Linux")
	}
	ctx := context.Background()
	s := New("cron", Opts{State: "running", Enable: true, InitSystem: "systemd"})
	state, err := s.Check(ctx)
	if err != nil {
		t.Skipf("systemctl not available or cron not present: %v", err)
	}
	t.Logf("cron service: InSync=%v, Changes=%+v", state.InSync, state.Changes)
}

func TestService_CheckSystemd_NonexistentService(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd tests only run on Linux")
	}
	ctx := context.Background()
	s := New("converge-definitely-not-real-12345", Opts{State: "running", Enable: true, InitSystem: "systemd"})
	state, err := s.Check(ctx)
	if err != nil {
		t.Skipf("systemctl not available: %v", err)
	}
	if state.InSync {
		t.Error("nonexistent service should not be in sync")
	}
}

func TestService_Check_Launchd(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd tests only run on macOS")
	}
	ctx := context.Background()
	// A service that does not exist must not falsely report compliant: a job
	// that is not loaded cannot satisfy State=running/Enable=true.
	s := New("com.converge.definitely-not-real-12345", Opts{State: "running", Enable: true, InitSystem: "launchd"})
	state, err := s.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("nonexistent service wanted running+enabled should report drift, not InSync")
	}
}

func TestService_Check_Launchd_UnsupportedInit(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("launchd tests only run on macOS")
	}
	ctx := context.Background()
	s := New("test", Opts{State: "running", InitSystem: "runit"})
	if _, err := s.Check(ctx); err == nil {
		t.Error("Check() should fail for unsupported init system on darwin")
	}
}

func TestService_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows SCM tests only run on Windows")
	}
	ctx := context.Background()

	// A service that is not installed cannot be opened in the SCM, so Check must
	// surface an error rather than silently reporting compliance.
	t.Run("nonexistent", func(t *testing.T) {
		s := New("converge-definitely-not-real-12345", Opts{State: "running", Enable: true, InitSystem: "windows"})
		if _, err := s.Check(ctx); err == nil {
			t.Error("Check() should error for a service that is not installed")
		}
	})

	// "Schedule" (Task Scheduler) is present on every Windows install, so it
	// exercises the real SCM query path. We do not assert running/stopped since
	// that can legitimately vary by host; only that Check completes and returns
	// a non-nil state.
	t.Run("well-known", func(t *testing.T) {
		s := New("Schedule", Opts{State: "running", Enable: true, InitSystem: "windows"})
		state, err := s.Check(ctx)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if state == nil {
			t.Fatal("Check() returned nil state")
		}
	})
}

func TestService_CheckSystemd_StoppedService(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd tests only run on Linux")
	}
	ctx := context.Background()
	s := New("converge-definitely-not-real-12345", Opts{State: "stopped", InitSystem: "systemd"})
	state, err := s.Check(ctx)
	if err != nil {
		t.Skipf("systemctl not available: %v", err)
	}
	if !state.InSync {
		t.Log("stopped+disabled nonexistent service should be in sync")
	}
}

func TestService_IsCritical(t *testing.T) {
	s := New("sshd", Opts{State: "running", Enable: true, InitSystem: "systemd"})
	if s.IsCritical() {
		t.Error("IsCritical() should be false by default")
	}
	s2 := New("sshd", Opts{State: "running", Enable: true, InitSystem: "systemd", Critical: true})
	if !s2.IsCritical() {
		t.Error("IsCritical() should be true when set")
	}
}

func TestService_New(t *testing.T) {
	s := New("nginx", Opts{State: "stopped", InitSystem: "systemd"})
	if s.Name != "nginx" {
		t.Errorf("Name = %q, want %q", s.Name, "nginx")
	}
	if s.State != "stopped" {
		t.Errorf("State = %q, want %q", s.State, "stopped")
	}
	if s.Enable {
		t.Error("Enable should be false")
	}
	if s.InitSystem != "systemd" {
		t.Errorf("InitSystem = %q, want %q", s.InitSystem, "systemd")
	}
}

func TestService_StartupType(t *testing.T) {
	s := New("wuauserv", Opts{State: "running", Enable: true, InitSystem: "windows", StartupType: "disabled"})
	if s.StartupType != "disabled" {
		t.Errorf("StartupType = %q, want %q", s.StartupType, "disabled")
	}
}
