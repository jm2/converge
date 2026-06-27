//go:build linux

package sysctl

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TsekNet/converge/extensions"
)

// TestSysctl_Watch_RejectsBadKey covers the input-validation guards in Watch
// that run before any inotify setup: explicit path traversal and a key that
// resolves outside the /proc/sys base.
func TestSysctl_Watch_RejectsBadKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
	}{
		{"path traversal", "net..ipv4.ip_forward"},
		{"escapes proc sys", ""}, // resolves to procSysBase itself, missing trailing slash
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := New(tt.key, Opts{Value: "1"})
			events := make(chan extensions.Event, 1)
			if err := s.Watch(context.Background(), events); err == nil {
				t.Errorf("Watch(key=%q) = nil, want error", tt.key)
			}
		})
	}
}

// TestSysctl_Watch_FiresEvent exercises the full inotify path: it redirects
// procSysBase at a temp directory (inotify works without root), watches a real
// file, modifies it, and asserts an EventWatch is delivered. Cancelling the
// context afterwards drives the ctx.Done return path in the watch loop.
//
// Not parallel: it mutates the package-global procSysBase.
func TestSysctl_Watch_FiresEvent(t *testing.T) {
	orig := procSysBase
	t.Cleanup(func() { procSysBase = orig })

	dir := t.TempDir()
	procSysBase = dir

	// Key "net.test.value" maps to <dir>/net/test/value.
	target := filepath.Join(dir, "net", "test", "value")
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("0\n"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan extensions.Event, 1)
	errCh := make(chan error, 1)
	go func() { errCh <- s_watch(ctx, target, events) }()

	// Repeatedly modify the file until an event lands (the watch goroutine may
	// not have registered the inotify watch on the very first write).
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()

	var got extensions.Event
	gotEvent := false
	for !gotEvent {
		select {
		case ev := <-events:
			got = ev
			gotEvent = true
		case <-deadline:
			t.Fatal("timed out waiting for inotify event")
		case <-tick.C:
			if err := os.WriteFile(target, []byte("1\n"), 0644); err != nil {
				t.Fatalf("modify file: %v", err)
			}
		}
	}

	if got.Kind != extensions.EventWatch {
		t.Errorf("event Kind = %v, want EventWatch", got.Kind)
	}
	if got.ResourceID != "sysctl:net.test.value" {
		t.Errorf("event ResourceID = %q, want %q", got.ResourceID, "sysctl:net.test.value")
	}
	if got.Detail != "inotify" {
		t.Errorf("event Detail = %q, want %q", got.Detail, "inotify")
	}

	// Cancelling should make Watch return nil via the ctx.Done branch.
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Watch returned %v, want nil after cancel", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after context cancel")
	}
}

// s_watch constructs a Sysctl whose key corresponds to target (under the
// currently-set procSysBase) and runs Watch. Helper to keep the test readable.
func s_watch(ctx context.Context, target string, events chan<- extensions.Event) error {
	rel, err := filepath.Rel(procSysBase, target)
	if err != nil {
		return err
	}
	key := filepath.ToSlash(rel)
	// Convert path separators back into the dotted sysctl key form.
	key = dotted(key)
	return New(key, Opts{Value: "1"}).Watch(ctx, events)
}

// dotted converts a slash-separated relative path into a dotted sysctl key.
func dotted(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			out = append(out, '.')
		} else {
			out = append(out, s[i])
		}
	}
	return string(out)
}
