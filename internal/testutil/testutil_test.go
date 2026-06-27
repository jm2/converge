package testutil

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/TsekNet/converge/extensions"
)

func TestMapFSSetGetHas(t *testing.T) {
	m := NewMapFS()

	if m.Has("/etc/foo") {
		t.Fatal("Has() on empty FS should be false")
	}

	m.Set("/etc/foo", []byte("hello"), 0o644)

	if !m.Has("/etc/foo") {
		t.Error("Has() should be true after Set")
	}
	got, ok := m.Get("/etc/foo")
	if !ok {
		t.Fatal("Get() should report ok after Set")
	}
	if string(got) != "hello" {
		t.Errorf("Get() = %q, want %q", got, "hello")
	}

	if _, ok := m.Get("/missing"); ok {
		t.Error("Get() on missing path should report !ok")
	}
}

func TestMapFSWriteReadFile(t *testing.T) {
	m := NewMapFS()

	if err := m.WriteFile("/a/b.txt", []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	got, err := m.ReadFile("/a/b.txt")
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	if string(got) != "data" {
		t.Errorf("ReadFile() = %q, want %q", got, "data")
	}

	// ReadFile returns a copy: mutating it must not affect storage.
	got[0] = 'X'
	again, _ := m.ReadFile("/a/b.txt")
	if string(again) != "data" {
		t.Errorf("ReadFile() not isolated, got %q", again)
	}

	// WriteFile copies input: mutating source must not affect storage.
	src := []byte("orig")
	if err := m.WriteFile("/c.txt", src, 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	src[0] = 'Z'
	stored, _ := m.ReadFile("/c.txt")
	if string(stored) != "orig" {
		t.Errorf("WriteFile() not isolated, got %q", stored)
	}

	if _, err := m.ReadFile("/nope"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile(missing) err = %v, want ErrNotExist", err)
	}
}

func TestMapFSStat(t *testing.T) {
	m := NewMapFS()
	m.Set("/dir/file.txt", []byte("abcde"), 0o640)

	fi, err := m.Stat("/dir/file.txt")
	if err != nil {
		t.Fatalf("Stat(): %v", err)
	}
	if fi.Name() != "file.txt" {
		t.Errorf("Name() = %q, want file.txt", fi.Name())
	}
	if fi.Size() != 5 {
		t.Errorf("Size() = %d, want 5", fi.Size())
	}
	if fi.Mode() != 0o640 {
		t.Errorf("Mode() = %v, want 0o640", fi.Mode())
	}
	if fi.IsDir() {
		t.Error("IsDir() should be false for a file")
	}
	if !fi.ModTime().IsZero() {
		t.Errorf("ModTime() = %v, want zero", fi.ModTime())
	}
	if fi.Sys() != nil {
		t.Errorf("Sys() = %v, want nil", fi.Sys())
	}

	if _, err := m.Stat("/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(missing) err = %v, want ErrNotExist", err)
	}
}

func TestMapFSChmod(t *testing.T) {
	m := NewMapFS()
	m.Set("/f", []byte("x"), 0o644)

	if err := m.Chmod("/f", 0o600); err != nil {
		t.Fatalf("Chmod(): %v", err)
	}
	fi, _ := m.Stat("/f")
	if fi.Mode() != 0o600 {
		t.Errorf("Mode() after Chmod = %v, want 0o600", fi.Mode())
	}

	if err := m.Chmod("/missing", 0o600); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Chmod(missing) err = %v, want ErrNotExist", err)
	}
}

func TestMapFSChownOwner(t *testing.T) {
	m := NewMapFS()
	m.Set("/f", []byte("x"), 0o644)

	if err := m.Chown("/f", 1000, 2000); err != nil {
		t.Fatalf("Chown(): %v", err)
	}
	uid, gid, err := m.Owner("/f")
	if err != nil {
		t.Fatalf("Owner(): %v", err)
	}
	if uid != 1000 || gid != 2000 {
		t.Errorf("Owner() = %d,%d, want 1000,2000", uid, gid)
	}

	// Negative values leave the existing owner untouched.
	if err := m.Chown("/f", -1, 3000); err != nil {
		t.Fatalf("Chown(): %v", err)
	}
	uid, gid, _ = m.Owner("/f")
	if uid != 1000 || gid != 3000 {
		t.Errorf("Owner() after partial chown = %d,%d, want 1000,3000", uid, gid)
	}

	if err := m.Chown("/missing", 1, 1); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Chown(missing) err = %v, want ErrNotExist", err)
	}
	if _, _, err := m.Owner("/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Owner(missing) err = %v, want ErrNotExist", err)
	}
}

func TestMapFSSetOwner(t *testing.T) {
	m := NewMapFS()
	m.Set("/f", []byte("x"), 0o644)

	m.SetOwner("/f", 42, 43)
	uid, gid, _ := m.Owner("/f")
	if uid != 42 || gid != 43 {
		t.Errorf("Owner() after SetOwner = %d,%d, want 42,43", uid, gid)
	}

	// SetOwner on a missing path is a silent no-op.
	m.SetOwner("/missing", 1, 1)
	if m.Has("/missing") {
		t.Error("SetOwner should not create a missing file")
	}
}

func TestMapFSMkdirAll(t *testing.T) {
	m := NewMapFS()

	if err := m.MkdirAll("/var/lib/converge", 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}

	for _, dir := range []string{"/var", "/var/lib", "/var/lib/converge"} {
		fi, err := m.Stat(dir)
		if err != nil {
			t.Fatalf("Stat(%q): %v", dir, err)
		}
		if !fi.IsDir() {
			t.Errorf("IsDir(%q) = false, want true", dir)
		}
		if fi.Mode()&fs.ModeDir == 0 {
			t.Errorf("Mode(%q) missing ModeDir bit: %v", dir, fi.Mode())
		}
	}

	// Re-running keeps existing entries (idempotent).
	if err := m.MkdirAll("/var/lib/converge", 0o755); err != nil {
		t.Fatalf("MkdirAll() repeat: %v", err)
	}
}

func TestMapFSRemove(t *testing.T) {
	m := NewMapFS()
	m.Set("/f", []byte("x"), 0o644)

	if err := m.Remove("/f"); err != nil {
		t.Fatalf("Remove(): %v", err)
	}
	if m.Has("/f") {
		t.Error("Has() should be false after Remove")
	}

	if err := m.Remove("/f"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove(missing) err = %v, want ErrNotExist", err)
	}
}

func TestMapFSSatisfiesInterface(t *testing.T) {
	// Compile-time check is in mapfs.go; exercise it at runtime too.
	var _ extensions.FS = NewMapFS()
}

func TestMockCmdSetOutputAndCommand(t *testing.T) {
	tests := []struct {
		name       string
		stdout     string
		err        error
		wantOutput string
		wantErr    bool
	}{
		{"success with stdout", "result line", nil, "result line", false},
		{"success empty stdout", "", nil, "", false},
		{"scripted error", "ignored", errors.New("boom"), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockCmd()
			m.SetOutput("tool", tt.stdout, tt.err)

			cmd := m.Command("tool", "arg1", "arg2")
			out, err := cmd.Output()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error from scripted command")
				}
				return
			}
			if err != nil {
				t.Fatalf("Output(): %v", err)
			}
			if string(out) != tt.wantOutput {
				t.Errorf("Output() = %q, want %q", out, tt.wantOutput)
			}
		})
	}
}

func TestMockCmdUnscripted(t *testing.T) {
	m := NewMockCmd()
	cmd := m.Command("unknown")
	if err := cmd.Run(); err == nil {
		t.Error("unscripted command should fail")
	}
	if !m.Called("unknown") {
		t.Error("Called() should record even unscripted invocations")
	}
}

func TestMockCmdCalledAndCallsFor(t *testing.T) {
	m := NewMockCmd()
	m.SetOutput("modprobe", "", nil)

	m.Command("modprobe", "overlay")
	m.Command("modprobe", "br_netfilter")
	m.Command("lsmod")

	if !m.Called("modprobe") {
		t.Error("Called(modprobe) = false, want true")
	}
	if m.Called("rmmod") {
		t.Error("Called(rmmod) = true, want false")
	}

	calls := m.CallsFor("modprobe")
	if len(calls) != 2 {
		t.Fatalf("CallsFor(modprobe) returned %d, want 2", len(calls))
	}
	if calls[0].Name != "modprobe" || len(calls[0].Args) != 1 || calls[0].Args[0] != "overlay" {
		t.Errorf("CallsFor[0] = %+v, want modprobe [overlay]", calls[0])
	}
	if calls[1].Args[0] != "br_netfilter" {
		t.Errorf("CallsFor[1] args = %v, want [br_netfilter]", calls[1].Args)
	}

	if got := m.CallsFor("nonexistent"); got != nil {
		t.Errorf("CallsFor(nonexistent) = %v, want nil", got)
	}
}

func TestMockCmdReset(t *testing.T) {
	m := NewMockCmd()
	m.SetOutput("tool", "out", nil)
	m.Command("tool")

	m.Reset()

	if m.Called("tool") {
		t.Error("Called() should be false after Reset")
	}
	if len(m.Calls) != 0 {
		t.Errorf("Calls len = %d after Reset, want 0", len(m.Calls))
	}
	// After Reset, the scripted output is also gone, so the command fails.
	if err := m.Command("tool").Run(); err == nil {
		t.Error("scripted output should be cleared by Reset")
	}
}

func TestMockCmdString(t *testing.T) {
	m := NewMockCmd()
	if s := m.String(); s != "" {
		t.Errorf("String() on empty = %q, want empty", s)
	}

	m.SetOutput("apt", "", nil)
	m.Command("apt", "install", "vim")
	s := m.String()
	if !strings.Contains(s, "[0]") || !strings.Contains(s, "apt") || !strings.Contains(s, "vim") {
		t.Errorf("String() = %q, missing expected content", s)
	}
}

func TestMockPackageManagerLifecycle(t *testing.T) {
	ctx := context.Background()
	m := NewMockPackageManager("apt")

	if m.Name() != "apt" {
		t.Errorf("Name() = %q, want apt", m.Name())
	}

	ok, err := m.IsInstalled(ctx, "vim")
	if err != nil {
		t.Fatalf("IsInstalled(): %v", err)
	}
	if ok {
		t.Error("IsInstalled() should be false initially")
	}

	if err := m.Install(ctx, "vim"); err != nil {
		t.Fatalf("Install(): %v", err)
	}
	ok, _ = m.IsInstalled(ctx, "vim")
	if !ok {
		t.Error("IsInstalled() should be true after Install")
	}

	if err := m.Remove(ctx, "vim"); err != nil {
		t.Fatalf("Remove(): %v", err)
	}
	ok, _ = m.IsInstalled(ctx, "vim")
	if ok {
		t.Error("IsInstalled() should be false after Remove")
	}
}

func TestMockPackageManagerWithInstallError(t *testing.T) {
	ctx := context.Background()
	m := NewMockPackageManager("dnf").WithInstallError("install failed")

	err := m.Install(ctx, "vim")
	if err == nil || err.Error() != "install failed" {
		t.Fatalf("Install() err = %v, want \"install failed\"", err)
	}
	// Install failed, so the package must not be marked installed.
	ok, _ := m.IsInstalled(ctx, "vim")
	if ok {
		t.Error("package should not be installed after a failed Install")
	}
}

func TestMockPackageManagerWithRemoveError(t *testing.T) {
	ctx := context.Background()
	m := NewMockPackageManager("dnf").WithRemoveError("remove failed")

	if err := m.Install(ctx, "vim"); err != nil {
		t.Fatalf("Install(): %v", err)
	}
	err := m.Remove(ctx, "vim")
	if err == nil || err.Error() != "remove failed" {
		t.Fatalf("Remove() err = %v, want \"remove failed\"", err)
	}
	// Remove failed, so the package stays installed.
	ok, _ := m.IsInstalled(ctx, "vim")
	if !ok {
		t.Error("package should remain installed after a failed Remove")
	}
}

// fakeExtension is a minimal extensions.Extension used to drive the assert
// helpers. It converges after a single Apply.
type fakeExtension struct {
	applied bool
	startIn bool // initial InSync value from Check
}

func (f *fakeExtension) ID() string     { return "fake" }
func (f *fakeExtension) String() string { return "fake extension" }

func (f *fakeExtension) Check(context.Context) (*extensions.State, error) {
	if f.applied || f.startIn {
		return &extensions.State{InSync: true}, nil
	}
	return &extensions.State{
		InSync:  false,
		Changes: []extensions.Change{{Property: "x", From: "a", To: "b", Action: "modify"}},
	}, nil
}

func (f *fakeExtension) Apply(context.Context) (*extensions.Result, error) {
	f.applied = true
	return &extensions.Result{Changed: true}, nil
}

func TestAssertConverges(t *testing.T) {
	AssertConverges(t, &fakeExtension{})
}

func TestAssertInSync(t *testing.T) {
	AssertInSync(t, &fakeExtension{startIn: true})
}

func TestAssertDrifted(t *testing.T) {
	AssertDrifted(t, &fakeExtension{})
}
