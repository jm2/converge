//go:build linux

package sysctl

import (
	"context"
	"testing"

	"github.com/TsekNet/converge/internal/testutil"
)

func TestSysctl_MapFS_CheckDetectsDrift(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mfs := testutil.NewMapFS()
	mfs.Set("/proc/sys/net/ipv4/ip_forward", []byte("0\n"), 0644)

	s := New("net.ipv4.ip_forward", Opts{Value: "1", FS: mfs})

	state, err := s.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if state.InSync {
		t.Error("Check() should report drift when value differs")
	}
	if len(state.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(state.Changes))
	}
	if state.Changes[0].From != "0" {
		t.Errorf("Changes[0].From = %q, want %q", state.Changes[0].From, "0")
	}
	if state.Changes[0].To != "1" {
		t.Errorf("Changes[0].To = %q, want %q", state.Changes[0].To, "1")
	}
}

func TestSysctl_MapFS_CheckInSync(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mfs := testutil.NewMapFS()
	mfs.Set("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0644)

	s := New("net.ipv4.ip_forward", Opts{Value: "1", FS: mfs})

	state, err := s.Check(ctx)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !state.InSync {
		t.Errorf("Check() should report in-sync, got changes: %+v", state.Changes)
	}
}

func TestSysctl_MapFS_Converges(t *testing.T) {
	t.Parallel()

	mfs := testutil.NewMapFS()
	mfs.Set("/proc/sys/net/ipv4/ip_forward", []byte("0\n"), 0644)

	s := New("net.ipv4.ip_forward", Opts{Value: "1", FS: mfs})
	testutil.AssertConverges(t, s)

	data, ok := mfs.Get("/proc/sys/net/ipv4/ip_forward")
	if !ok {
		t.Fatal("file should exist in MapFS after Apply")
	}
	if string(data) != "1\n" {
		t.Errorf("content = %q, want %q", data, "1\n")
	}
}

func TestSysctl_MapFS_ApplyWithPersist(t *testing.T) {
	t.Parallel()

	mfs := testutil.NewMapFS()
	mfs.Set("/proc/sys/kernel/randomize_va_space", []byte("1\n"), 0644)

	s := New("kernel.randomize_va_space", Opts{Value: "2", Persist: true, FS: mfs})
	testutil.AssertConverges(t, s)

	data, ok := mfs.Get("/etc/sysctl.d/99-converge.conf")
	if !ok {
		t.Fatal("persist file should exist in MapFS after Apply")
	}
	if string(data) != "kernel.randomize_va_space = 2\n" {
		t.Errorf("persist content = %q, want %q", data, "kernel.randomize_va_space = 2\n")
	}
}

func TestSysctl_MapFS_PersistReadModifyWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mfs := testutil.NewMapFS()
	// Pre-seed the persist file with an existing entry.
	mfs.Set("/etc/sysctl.d/99-converge.conf", []byte("net.ipv4.ip_forward = 1\n"), 0644)
	mfs.Set("/proc/sys/kernel/randomize_va_space", []byte("1\n"), 0644)

	s := New("kernel.randomize_va_space", Opts{Value: "2", Persist: true, FS: mfs})
	_, err := s.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	data, ok := mfs.Get("/etc/sysctl.d/99-converge.conf")
	if !ok {
		t.Fatal("persist file should exist")
	}
	want := "net.ipv4.ip_forward = 1\nkernel.randomize_va_space = 2\n"
	if string(data) != want {
		t.Errorf("persist content =\n%q\nwant\n%q", data, want)
	}
}

func TestSysctl_MapFS_PersistUpdatesExistingKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mfs := testutil.NewMapFS()
	mfs.Set("/etc/sysctl.d/99-converge.conf", []byte("net.ipv4.ip_forward = 0\nkernel.randomize_va_space = 1\n"), 0644)
	mfs.Set("/proc/sys/net/ipv4/ip_forward", []byte("0\n"), 0644)

	s := New("net.ipv4.ip_forward", Opts{Value: "1", Persist: true, FS: mfs})
	_, err := s.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	data, ok := mfs.Get("/etc/sysctl.d/99-converge.conf")
	if !ok {
		t.Fatal("persist file should exist")
	}
	want := "net.ipv4.ip_forward = 1\nkernel.randomize_va_space = 1\n"
	if string(data) != want {
		t.Errorf("persist content =\n%q\nwant\n%q", data, want)
	}
}

func TestSysctl_MapFS_ValidateRejectsInvalidKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mfs := testutil.NewMapFS()
	s := New("net..ipv4", Opts{Value: "1", FS: mfs})

	_, err := s.Check(ctx)
	if err == nil {
		t.Error("Check() should reject key with '..'")
	}

	_, err = s.Apply(ctx)
	if err == nil {
		t.Error("Apply() should reject key with '..'")
	}
}
