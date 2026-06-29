//go:build linux

package watch

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestEscapeUnitName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"alphanumeric only", "sshd", "sshd"},
		{"dot escaped", "sshd.service", "sshd_2eservice"},
		{"dash escaped", "my-unit.service", "my_2dunit_2eservice"},
		{"at and digits", "getty@tty1.service", "getty_40tty1_2eservice"},
		{"empty", "", ""},
		{"uppercase preserved", "ABC", "ABC"},
		{"slash escaped", "a/b", "a_2fb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeUnitName(tt.in); got != tt.want {
				t.Errorf("escapeUnitName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestUnitObjectPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want dbus.ObjectPath
	}{
		{
			name: "service unit",
			in:   "sshd.service",
			want: dbus.ObjectPath("/org/freedesktop/systemd1/unit/sshd_2eservice"),
		},
		{
			name: "templated unit",
			in:   "getty@tty1.service",
			want: dbus.ObjectPath("/org/freedesktop/systemd1/unit/getty_40tty1_2eservice"),
		},
		{
			name: "plain name",
			in:   "cron",
			want: dbus.ObjectPath("/org/freedesktop/systemd1/unit/cron"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unitObjectPath(tt.in); got != tt.want {
				t.Errorf("unitObjectPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// newTestDbusWatcher builds a DbusWatcher with initialized maps but no live
// connection. readLoop only touches mu/pathSubs, so it can be exercised
// without a system bus.
func newTestDbusWatcher() *DbusWatcher {
	return &DbusWatcher{
		pathSubs:   make(map[dbus.ObjectPath][]*dbusSubscriber),
		unitToPath: make(map[string]dbus.ObjectPath),
		done:       make(chan struct{}),
	}
}

func TestDbusReadLoopActiveStateChange(t *testing.T) {
	w := newTestDbusWatcher()
	path := unitObjectPath("sshd.service")
	ch := make(chan struct{}, 1)
	w.pathSubs[path] = []*dbusSubscriber{{ch: ch}}

	signals := make(chan *dbus.Signal, 4)
	go w.readLoop(signals)

	signals <- &dbus.Signal{
		Path: path,
		Body: []interface{}{
			"org.freedesktop.systemd1.Unit",
			map[string]dbus.Variant{
				"ActiveState": dbus.MakeVariant("active"),
			},
			[]string{},
		},
	}

	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ActiveState notification")
	}
	close(signals)
}

func TestDbusReadLoopSubStateChange(t *testing.T) {
	w := newTestDbusWatcher()
	path := unitObjectPath("nginx.service")
	ch := make(chan struct{}, 1)
	w.pathSubs[path] = []*dbusSubscriber{{ch: ch}}

	signals := make(chan *dbus.Signal, 4)
	go w.readLoop(signals)

	signals <- &dbus.Signal{
		Path: path,
		Body: []interface{}{
			"org.freedesktop.systemd1.Unit",
			map[string]dbus.Variant{
				"SubState": dbus.MakeVariant("running"),
			},
			[]string{},
		},
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SubState notification")
	}
	close(signals)
}

func TestDbusReadLoopFiltersIrrelevantSignals(t *testing.T) {
	w := newTestDbusWatcher()
	path := unitObjectPath("sshd.service")
	ch := make(chan struct{}, 1)
	w.pathSubs[path] = []*dbusSubscriber{{ch: ch}}

	signals := make(chan *dbus.Signal, 8)
	go w.readLoop(signals)

	// nil signal.
	signals <- nil
	// Too few body elements.
	signals <- &dbus.Signal{Path: path, Body: []interface{}{"only-one"}}
	// Wrong first-arg type.
	signals <- &dbus.Signal{Path: path, Body: []interface{}{42, map[string]dbus.Variant{}}}
	// Wrong interface.
	signals <- &dbus.Signal{Path: path, Body: []interface{}{
		"org.freedesktop.systemd1.Service",
		map[string]dbus.Variant{"ActiveState": dbus.MakeVariant("active")},
	}}
	// Right interface but changed map wrong type.
	signals <- &dbus.Signal{Path: path, Body: []interface{}{
		"org.freedesktop.systemd1.Unit",
		"not-a-map",
	}}
	// Right interface, changed map without ActiveState/SubState.
	signals <- &dbus.Signal{Path: path, Body: []interface{}{
		"org.freedesktop.systemd1.Unit",
		map[string]dbus.Variant{"Description": dbus.MakeVariant("x")},
	}}

	select {
	case <-ch:
		t.Fatal("received notification for a filtered-out signal")
	case <-time.After(300 * time.Millisecond):
	}
	close(signals)
}

func TestDbusReadLoopUnknownPathIgnored(t *testing.T) {
	w := newTestDbusWatcher()
	// No subscribers registered for the signal's path.
	signals := make(chan *dbus.Signal, 2)
	go w.readLoop(signals)

	signals <- &dbus.Signal{
		Path: unitObjectPath("unknown.service"),
		Body: []interface{}{
			"org.freedesktop.systemd1.Unit",
			map[string]dbus.Variant{"ActiveState": dbus.MakeVariant("active")},
		},
	}
	// Just ensure the loop processes and exits cleanly on close.
	time.Sleep(100 * time.Millisecond)
	close(signals)
}

// TestUnwatchUnitUnknownUnit exercises the early-return path of UnwatchUnit,
// which does not require a live D-Bus connection.
func TestUnwatchUnitUnknownUnit(t *testing.T) {
	w := newTestDbusWatcher()
	// Should be a no-op and not panic (conn is nil but never dereferenced).
	w.UnwatchUnit("never-watched.service", make(chan struct{}))
}

// TestSharedDbusUnavailable verifies SharedDbus surfaces an error gracefully
// when no system bus is reachable. It is skipped when a system bus exists.
func TestSharedDbusUnavailable(t *testing.T) {
	w, err := SharedDbus()
	if err != nil {
		// No system bus: error path. Watcher must be nil.
		if w != nil {
			t.Error("expected nil watcher when SharedDbus fails")
		}
		return
	}
	// A system bus is present (e.g. running under systemd). Memoization must
	// return the same instance on a second call.
	w2, err2 := SharedDbus()
	if err2 != nil {
		t.Fatalf("second SharedDbus call returned error: %v", err2)
	}
	if w != w2 {
		t.Error("SharedDbus did not return memoized instance")
	}
}
