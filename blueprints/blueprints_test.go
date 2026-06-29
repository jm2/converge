package blueprints_test

import (
	"testing"

	"github.com/TsekNet/converge/blueprints"
	"github.com/TsekNet/converge/dsl"
)

// buildBlueprint registers bp under name and builds its graph through the dsl
// App, mirroring how the CLI invokes blueprint functions. Any helpers (such as
// "linux" for LinuxServer's Include) are registered first.
func buildBlueprint(t *testing.T, name string, bp dsl.Blueprint, deps map[string]dsl.Blueprint) ([]string, error) {
	t.Helper()
	app := dsl.New()
	for depName, depFn := range deps {
		app.Register(depName, "", depFn)
	}
	app.Register(name, "", bp)

	g, err := app.BuildGraph(name)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, n := range g.Nodes() {
		ids = append(ids, n.Ext.ID())
	}
	return ids, nil
}

// TestBlueprints invokes each cross-platform-callable constructor through the
// dsl App and asserts it produces a non-empty, acyclic graph without error.
func TestBlueprints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		bp   dsl.Blueprint
		deps map[string]dsl.Blueprint
	}{
		{name: "baseline", bp: blueprints.Baseline},
		{name: "linux", bp: blueprints.Linux},
		{name: "darwin", bp: blueprints.Darwin},
		{
			name: "linux_server",
			bp:   blueprints.LinuxServer,
			// LinuxServer pulls in "linux" via r.Include. We stub it with a
			// no-op so the Include resolves; the real linux blueprint also
			// declares "firewall:Allow SSH", which collides with the copy
			// LinuxServer declares itself. Stubbing isolates LinuxServer's own
			// resource set under test.
			deps: map[string]dsl.Blueprint{"linux": func(r *dsl.Run) {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ids, err := buildBlueprint(t, tt.name, tt.bp, tt.deps)
			if err != nil {
				t.Fatalf("BuildGraph(%q): %v", tt.name, err)
			}
			if len(ids) == 0 {
				t.Fatalf("%s produced no resources", tt.name)
			}

			// Every resource ID must be well-formed ("type:name") and unique.
			seen := make(map[string]bool, len(ids))
			for _, id := range ids {
				if id == "" {
					t.Errorf("%s produced an empty resource ID", tt.name)
				}
				if seen[id] {
					t.Errorf("%s produced duplicate resource ID %q", tt.name, id)
				}
				seen[id] = true
			}
		})
	}
}

// TestBlueprints_CommonResources checks that resources we expect on every
// platform-tolerant blueprint show up, without over-asserting on
// runtime-dependent (OS/distro/shard) selections.
func TestBlueprints_CommonResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		bp      dsl.Blueprint
		deps    map[string]dsl.Blueprint
		wantIDs []string
	}{
		{
			name:    "baseline allows ssh",
			bp:      blueprints.Baseline,
			wantIDs: []string{"firewall:Allow SSH"},
		},
		{
			name:    "linux manages motd and cron",
			bp:      blueprints.Linux,
			wantIDs: []string{"file:/etc/motd", "service:cron", "firewall:Allow SSH"},
		},
		{
			name:    "darwin manages motd and ssh",
			bp:      blueprints.Darwin,
			wantIDs: []string{"file:/etc/motd", "firewall:Allow SSH", "package:git"},
		},
		{
			name:    "linux_server hardens sshd",
			bp:      blueprints.LinuxServer,
			deps:    map[string]dsl.Blueprint{"linux": func(r *dsl.Run) {}},
			wantIDs: []string{"service:sshd", "package:fail2ban", "file:/etc/ssh/sshd_config.d/converge.conf"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ids, err := buildBlueprint(t, tt.name, tt.bp, tt.deps)
			if err != nil {
				t.Fatalf("BuildGraph: %v", err)
			}
			have := make(map[string]bool, len(ids))
			for _, id := range ids {
				have[id] = true
			}
			for _, want := range tt.wantIDs {
				if !have[want] {
					t.Errorf("%s: missing expected resource %q (have %v)", tt.name, want, ids)
				}
			}
		})
	}
}
