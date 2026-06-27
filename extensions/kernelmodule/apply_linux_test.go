//go:build linux

package kernelmodule

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TsekNet/converge/internal/testutil"
)

// withMockCmd swaps newCommand for the duration of the test.
func withMockCmd(t *testing.T, mock *testutil.MockCmd) {
	t.Helper()
	old := newCommand
	newCommand = mock.Command
	t.Cleanup(func() { newCommand = old })
}

// withModprobeDir redirects modprobeDir to a temp dir for the test.
func withModprobeDir(t *testing.T, dir string) {
	t.Helper()
	old := modprobeDir
	modprobeDir = dir
	t.Cleanup(func() { modprobeDir = old })
}

// withProcModules redirects procModules to a temp file containing the given
// /proc/modules-style contents, and returns nothing.
func withProcModules(t *testing.T, contents string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "modules")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write proc modules: %v", err)
	}
	old := procModules
	procModules = path
	t.Cleanup(func() { procModules = old })
}

func TestModprobe(t *testing.T) {
	tests := []struct {
		name     string
		module   string
		remove   bool
		cmdErr   error
		wantArgs []string
		wantErr  bool
	}{
		{name: "load success", module: "ext4", remove: false, wantArgs: []string{"ext4"}},
		{name: "remove success", module: "ext4", remove: true, wantArgs: []string{"-r", "ext4"}},
		{name: "load failure", module: "ext4", remove: false, cmdErr: errors.New("boom"), wantArgs: []string{"ext4"}, wantErr: true},
		{name: "remove failure", module: "ext4", remove: true, cmdErr: errors.New("boom"), wantArgs: []string{"-r", "ext4"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockCmd()
			mock.SetOutput("/sbin/modprobe", "", tt.cmdErr)
			withMockCmd(t, mock)

			err := modprobe(tt.module, tt.remove)
			if (err != nil) != tt.wantErr {
				t.Fatalf("modprobe() error = %v, wantErr %v", err, tt.wantErr)
			}

			calls := mock.CallsFor("/sbin/modprobe")
			if len(calls) != 1 {
				t.Fatalf("expected 1 modprobe call, got %d", len(calls))
			}
			if strings.Join(calls[0].Args, " ") != strings.Join(tt.wantArgs, " ") {
				t.Errorf("modprobe args = %v, want %v", calls[0].Args, tt.wantArgs)
			}
		})
	}
}

func TestLoadModule(t *testing.T) {
	mock := testutil.NewMockCmd()
	mock.SetOutput("/sbin/modprobe", "", nil)
	withMockCmd(t, mock)

	if err := loadModule("ext4"); err != nil {
		t.Fatalf("loadModule() error: %v", err)
	}
	calls := mock.CallsFor("/sbin/modprobe")
	if len(calls) != 1 || strings.Join(calls[0].Args, " ") != "ext4" {
		t.Errorf("loadModule should run 'modprobe ext4', got %v", calls)
	}
}

func TestUnloadModule(t *testing.T) {
	mock := testutil.NewMockCmd()
	mock.SetOutput("/sbin/modprobe", "", nil)
	withMockCmd(t, mock)

	if err := unloadModule("ext4"); err != nil {
		t.Fatalf("unloadModule() error: %v", err)
	}
	calls := mock.CallsFor("/sbin/modprobe")
	if len(calls) != 1 || strings.Join(calls[0].Args, " ") != "-r ext4" {
		t.Errorf("unloadModule should run 'modprobe -r ext4', got %v", calls)
	}
}

func TestAddToBlacklist_RealFile(t *testing.T) {
	dir := t.TempDir()
	withModprobeDir(t, dir)
	path := filepath.Join(dir, blacklistFile)

	// First add creates the file with the entry.
	if err := addToBlacklist("cramfs"); err != nil {
		t.Fatalf("addToBlacklist() error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read blacklist: %v", err)
	}
	if !strings.Contains(string(data), "blacklist cramfs") {
		t.Errorf("blacklist file missing entry, got %q", data)
	}

	// Second add of the same module is idempotent (no duplicate).
	if err := addToBlacklist("cramfs"); err != nil {
		t.Fatalf("addToBlacklist() second call error: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read blacklist: %v", err)
	}
	if got := strings.Count(string(data), "blacklist cramfs"); got != 1 {
		t.Errorf("expected exactly 1 entry, got %d in %q", got, data)
	}

	// Adding a different module appends a new entry.
	if err := addToBlacklist("freevxfs"); err != nil {
		t.Fatalf("addToBlacklist() error: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read blacklist: %v", err)
	}
	if !strings.Contains(string(data), "blacklist freevxfs") {
		t.Errorf("blacklist file missing second entry, got %q", data)
	}

	// Now isModuleBlacklisted should report true for both.
	for _, mod := range []string{"cramfs", "freevxfs"} {
		bl, err := isModuleBlacklisted(mod)
		if err != nil {
			t.Fatalf("isModuleBlacklisted(%s) error: %v", mod, err)
		}
		if !bl {
			t.Errorf("isModuleBlacklisted(%s) = false, want true", mod)
		}
	}
}

func TestAddToBlacklist_MkdirCreatesDir(t *testing.T) {
	// modprobeDir does not exist yet; addToBlacklist should MkdirAll it.
	dir := filepath.Join(t.TempDir(), "nested", "modprobe.d")
	withModprobeDir(t, dir)

	if err := addToBlacklist("cramfs"); err != nil {
		t.Fatalf("addToBlacklist() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, blacklistFile)); err != nil {
		t.Errorf("blacklist file not created: %v", err)
	}
}

func TestRemoveFromBlacklist_RealFile(t *testing.T) {
	dir := t.TempDir()
	withModprobeDir(t, dir)
	path := filepath.Join(dir, blacklistFile)

	content := "blacklist cramfs\nblacklist freevxfs\nblacklist jffs2\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("seed blacklist: %v", err)
	}

	if err := removeFromBlacklist("freevxfs"); err != nil {
		t.Fatalf("removeFromBlacklist() error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "blacklist freevxfs") {
		t.Errorf("removed module still present: %q", data)
	}
	if !strings.Contains(string(data), "blacklist cramfs") || !strings.Contains(string(data), "blacklist jffs2") {
		t.Errorf("other modules lost: %q", data)
	}
}

func TestRemoveFromBlacklist_NoFile(t *testing.T) {
	// No blacklist file present: removal is a no-op returning nil.
	withModprobeDir(t, t.TempDir())
	if err := removeFromBlacklist("cramfs"); err != nil {
		t.Errorf("removeFromBlacklist() with no file should be nil, got %v", err)
	}
}

func TestIsModuleLoaded_RealFile(t *testing.T) {
	withProcModules(t, "ext4 1000 1 - Live 0x0000000000000000\nsnd_hda_intel 2000 0 - Live 0x0\n")

	tests := []struct {
		name   string
		module string
		want   bool
	}{
		{"loaded", "ext4", true},
		{"loaded underscore", "snd_hda_intel", true},
		{"hyphen normalized", "snd-hda-intel", true},
		{"not loaded", "cramfs", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isModuleLoaded(tt.module)
			if err != nil {
				t.Fatalf("isModuleLoaded() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("isModuleLoaded(%s) = %v, want %v", tt.module, got, tt.want)
			}
		})
	}
}

func TestIsModuleLoaded_OpenError(t *testing.T) {
	old := procModules
	procModules = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(func() { procModules = old })

	if _, err := isModuleLoaded("ext4"); err == nil {
		t.Error("isModuleLoaded() should error when /proc/modules is missing")
	}
}

func TestApply_Loaded(t *testing.T) {
	withModprobeDir(t, t.TempDir())
	withProcModules(t, "") // module not currently loaded
	mock := testutil.NewMockCmd()
	mock.SetOutput("/sbin/modprobe", "", nil)
	withMockCmd(t, mock)

	k := New("ext4", Opts{State: Loaded})
	res, err := k.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if !res.Changed || res.Message != "loaded" {
		t.Errorf("Apply() = %+v, want changed loaded", res)
	}
	// Should have run modprobe to load (no -r).
	calls := mock.CallsFor("/sbin/modprobe")
	if len(calls) != 1 || strings.Join(calls[0].Args, " ") != "ext4" {
		t.Errorf("expected 'modprobe ext4', got %v", calls)
	}
}

func TestApply_Loaded_AlreadyLoaded(t *testing.T) {
	withModprobeDir(t, t.TempDir())
	withProcModules(t, "ext4 1000 1 - Live 0x0\n") // already loaded
	mock := testutil.NewMockCmd()
	mock.SetOutput("/sbin/modprobe", "", nil)
	withMockCmd(t, mock)

	k := New("ext4", Opts{State: Loaded})
	res, err := k.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if !res.Changed {
		t.Errorf("Apply() should report changed (blacklist removal path), got %+v", res)
	}
	// Already loaded: modprobe load must NOT be called.
	if mock.Called("/sbin/modprobe") {
		t.Errorf("modprobe should not run when module already loaded: %v", mock.CallsFor("/sbin/modprobe"))
	}
}

func TestApply_Loaded_LoadFails(t *testing.T) {
	withModprobeDir(t, t.TempDir())
	withProcModules(t, "")
	mock := testutil.NewMockCmd()
	mock.SetOutput("/sbin/modprobe", "", errors.New("modprobe failed"))
	withMockCmd(t, mock)

	k := New("ext4", Opts{State: Loaded})
	if _, err := k.Apply(context.Background()); err == nil {
		t.Error("Apply() should fail when modprobe load fails")
	}
}

func TestApply_Blacklisted(t *testing.T) {
	dir := t.TempDir()
	withModprobeDir(t, dir)
	withProcModules(t, "cramfs 1000 0 - Live 0x0\n") // currently loaded
	mock := testutil.NewMockCmd()
	mock.SetOutput("/sbin/modprobe", "", nil)
	withMockCmd(t, mock)

	k := New("cramfs", Opts{State: Blacklisted})
	res, err := k.Apply(context.Background())
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if !res.Changed || res.Message != "blacklisted" {
		t.Errorf("Apply() = %+v, want changed blacklisted", res)
	}
	// Loaded module gets unloaded with -r.
	calls := mock.CallsFor("/sbin/modprobe")
	if len(calls) != 1 || strings.Join(calls[0].Args, " ") != "-r cramfs" {
		t.Errorf("expected 'modprobe -r cramfs', got %v", calls)
	}
	// And the blacklist entry is written.
	data, _ := os.ReadFile(filepath.Join(dir, blacklistFile))
	if !strings.Contains(string(data), "blacklist cramfs") {
		t.Errorf("blacklist entry not written: %q", data)
	}
}

func TestApply_Blacklisted_NotLoaded(t *testing.T) {
	dir := t.TempDir()
	withModprobeDir(t, dir)
	withProcModules(t, "") // not loaded
	mock := testutil.NewMockCmd()
	mock.SetOutput("/sbin/modprobe", "", nil)
	withMockCmd(t, mock)

	k := New("cramfs", Opts{State: Blacklisted})
	if _, err := k.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	// Not loaded: no unload should happen.
	if mock.Called("/sbin/modprobe") {
		t.Errorf("modprobe -r should not run when module not loaded: %v", mock.CallsFor("/sbin/modprobe"))
	}
	data, _ := os.ReadFile(filepath.Join(dir, blacklistFile))
	if !strings.Contains(string(data), "blacklist cramfs") {
		t.Errorf("blacklist entry not written: %q", data)
	}
}

func TestApply_Blacklisted_UnloadFails(t *testing.T) {
	withModprobeDir(t, t.TempDir())
	withProcModules(t, "cramfs 1000 0 - Live 0x0\n")
	mock := testutil.NewMockCmd()
	mock.SetOutput("/sbin/modprobe", "", errors.New("in use"))
	withMockCmd(t, mock)

	k := New("cramfs", Opts{State: Blacklisted})
	if _, err := k.Apply(context.Background()); err == nil {
		t.Error("Apply() should fail when unload fails")
	}
}

func TestApply_InvalidState(t *testing.T) {
	withProcModules(t, "")
	k := &KernelModule{Module: "cramfs", State: "bogus"}
	if _, err := k.Apply(context.Background()); err == nil {
		t.Error("Apply() should fail with unknown state")
	}
}

func TestApply_InvalidModuleName(t *testing.T) {
	k := &KernelModule{Module: "bad name", State: Loaded}
	if _, err := k.Apply(context.Background()); err == nil {
		t.Error("Apply() should fail validation for invalid module name")
	}
}

func TestPollInterval(t *testing.T) {
	k := New("ext4", Opts{State: Loaded})
	if got := k.PollInterval(); got <= 0 {
		t.Errorf("PollInterval() = %v, want positive", got)
	}
}
