package output

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TsekNet/converge/extensions"
)

func TestSupportsColor(t *testing.T) {
	t.Run("no_color_set", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		if SupportsColor() {
			t.Error("SupportsColor() = true with NO_COLOR set, want false")
		}
	})

	t.Run("not_a_tty", func(t *testing.T) {
		// During `go test` stdout is a pipe, not a character device, so
		// isTTY() is false and SupportsColor must return false even when
		// NO_COLOR is unset.
		if orig, ok := os.LookupEnv("NO_COLOR"); ok {
			os.Unsetenv("NO_COLOR")
			t.Cleanup(func() { os.Setenv("NO_COLOR", orig) })
		}
		if SupportsColor() {
			t.Error("SupportsColor() = true on non-TTY stdout, want false")
		}
	})
}

func TestEnableVT(t *testing.T) {
	// enableVT is a bool-returning no-op/feature-probe; just ensure it runs.
	_ = enableVT()
}

func TestIsTTY_NonTTY(t *testing.T) {
	// Under `go test` stdout is redirected, so isTTY should report false.
	if isTTY() {
		t.Skip("stdout is a TTY in this environment; skipping non-TTY assertion")
	}
}

func TestSpinner_StopInternal(t *testing.T) {
	t.Run("active", func(t *testing.T) {
		s := NewSpinner()
		s.active = true
		s.stopCh = make(chan struct{})
		s.doneCh = make(chan struct{})
		// Simulate the spinner goroutine: close doneCh once stopCh is closed.
		go func() {
			<-s.stopCh
			close(s.doneCh)
		}()
		s.stopInternal()
		if s.active {
			t.Error("stopInternal() left spinner active")
		}
	})

	t.Run("not_active", func(t *testing.T) {
		s := NewSpinner()
		// Must be a no-op (no panic, stays inactive) when not running.
		s.stopInternal()
		if s.active {
			t.Error("stopInternal() activated an inactive spinner")
		}
	})
}

func TestSpinner_StopWhenInactive(t *testing.T) {
	s := NewSpinner()
	// Stop on a never-started spinner returns immediately without panicking.
	s.Stop()
}

func TestTerminalPrinter_ApplyResult_Changed(t *testing.T) {
	ext := &stubExt{id: "file:/etc/test", name: "File /etc/test"}
	result := &extensions.Result{
		Changed: true,
		Status:  extensions.StatusChanged,
		Message: "updated",
		Changes: []extensions.Change{
			{Property: "content", From: "old", To: "new", Action: "modify"},
			{Property: "mode", To: "0644", Action: "add"},
			{Property: "owner", From: "root", Action: "remove"},
		},
		Duration: 10 * time.Millisecond,
	}
	out := captureStdout(t, func() {
		p := NewTerminalPrinter()
		p.SetMaxNameLen(20)
		p.ApplyResult(ext, result)
	})
	if !strings.Contains(out, "content") {
		t.Errorf("ApplyResult() output missing change detail:\n%s", out)
	}
	if !strings.Contains(out, "→") {
		t.Errorf("ApplyResult() output missing modify arrow:\n%s", out)
	}
}

func TestSerialPrinter_ApplyResult_Changed(t *testing.T) {
	ext := &stubExt{id: "file:/etc/test", name: "File /etc/test"}
	result := &extensions.Result{
		Changed: true,
		Status:  extensions.StatusChanged,
		Message: "updated",
		Changes: []extensions.Change{
			{Property: "content", From: "old", To: "new", Action: "modify"},
			{Property: "mode", To: "0644", Action: "add"},
			{Property: "owner", From: "root", Action: "remove"},
		},
		Duration: 10 * time.Millisecond,
	}
	out := captureStdout(t, func() {
		p := NewSerialPrinter()
		p.SetMaxNameLen(20)
		p.ApplyResult(ext, result)
	})
	if !strings.Contains(out, "~ /etc/test") {
		t.Errorf("ApplyResult() output missing changed marker:\n%s", out)
	}
	if !strings.Contains(out, "content: old -> new") {
		t.Errorf("ApplyResult() output missing modify detail:\n%s", out)
	}
}

func TestFormatDuration_Negative(t *testing.T) {
	if got := formatDuration(-5 * time.Millisecond); got != "0ms" {
		t.Errorf("formatDuration(negative) = %q, want %q", got, "0ms")
	}
}

// Exercise the no-op printer methods so intent is documented; these have no
// executable statements but are part of the Printer interface contract.
func TestNoOpPrinterMethods(t *testing.T) {
	ext := &stubExt{id: "file:/a", name: "File /a"}

	jp := NewJSONPrinter()
	jp.SetMaxNameLen(10)
	jp.Banner("v")
	jp.ResourceChecking(ext, 1, 2)
	jp.ApplyStart(ext, 1, 2)

	sp := NewSerialPrinter()
	sp.ApplyStart(ext, 1, 2)
}
