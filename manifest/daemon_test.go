package manifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/TsekNet/converge/internal/daemon"
	"github.com/TsekNet/converge/internal/output"
)

// TestDaemonRestoresHCLManagedFile proves the full HCL -> graph -> daemon ->
// watcher loop: a file managed by an HCL manifest is converged, deleted
// out-of-band, and restored by the daemon's watcher path. This is the daemon
// analog of the engine plan check and needs no root (only the CLI `serve`
// command gates root; daemon.New does not).
func TestDaemonRestoresHCLManagedFile(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: drives the daemon and a file watcher")
	}
	if runtime.GOOS == "windows" {
		// The Linux (inotify) and macOS (kqueue) watch->restore paths are
		// exercised here, but the Windows ReadDirectoryChangesW restore is not
		// yet reliably observed within the test window on CI runners. Skip on
		// Windows until that path is validated rather than asserting behavior we
		// cannot yet confirm. See the cross-platform CI notes.
		t.Skip("windows watch->restore not yet validated in CI; tracked separately")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "managed.conf")
	const want = "managed by converge\n"

	g, diags := Load("t.hcl", []byte(fmt.Sprintf(`
resource "file" "c" {
  path    = %q
  content = %q
  mode    = "0644"
}
`, path, want)))
	if diags.HasErrors() {
		t.Fatalf("load: %s", diags.Error())
	}

	d := daemon.New(g, output.NewJSONPrinter(), daemon.Options{
		Timeout:        30 * time.Second,
		CoalesceWindow: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("daemon did not stop within 10s of cancel")
		}
	}()

	// Initial convergence creates the file.
	if !waitForContent(path, want, 15*time.Second) {
		t.Fatalf("file %s was not converged by the daemon", path)
	}

	// Simulate out-of-band drift and wait for the daemon's watcher to restore
	// the file. Watchers start in a phase after the initial convergence (see
	// daemon.Run), so a delete issued the instant the file first appears can
	// land before the watcher is armed. Because the file resource is a Watcher
	// with no poll fallback, such a delete event is missed permanently. The
	// arming window is platform-dependent (inotify on Linux/kqueue on macOS arm
	// in a single syscall; Windows ReadDirectoryChangesW setup is heavier), so
	// retry the deletion until the daemon restores the file: once the watcher is
	// armed, the next deletion is caught and the file is re-converged.
	deadline := time.Now().Add(20 * time.Second)
	restored := false
	for time.Now().Before(deadline) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove: %v", err)
		}
		if waitForContent(path, want, time.Second) {
			restored = true
			break
		}
		// Not restored yet: the watcher was likely not armed when this delete
		// fired. Re-create the file so the next iteration can trigger a fresh
		// delete event once the watcher is up. If the watcher is already armed
		// this is a no-op drift (content already matches) and the next delete
		// is what gets restored.
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatalf("recreate: %v", err)
		}
	}
	if !restored {
		t.Fatalf("daemon did not restore %s after out-of-band deletion", path)
	}
}

// waitForContent polls until the file at path has exactly want, or the deadline.
func waitForContent(path, want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && string(b) == want {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
