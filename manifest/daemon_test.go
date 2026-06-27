package manifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

	// Simulate out-of-band drift: delete the managed file.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// The watcher must detect the deletion and restore the file.
	if !waitForContent(path, want, 15*time.Second) {
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
