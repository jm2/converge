//go:build linux

package cis

import (
	"slices"
	"testing"

	"github.com/TsekNet/converge/dsl"
)

// buildCIS registers LinuxCIS and builds its dependency graph the same way the
// CLI does, exercising LinuxCIS and every cis* sub-function transitively.
func buildCIS(t *testing.T) []string {
	t.Helper()

	app := dsl.New()
	app.Register("cis", "CIS Ubuntu 24.04 L1 Server", LinuxCIS)

	g, err := app.BuildGraph("cis")
	if err != nil {
		t.Fatalf("BuildGraph(cis) returned error: %v", err)
	}

	exts := g.OrderedExtensions()
	if len(exts) == 0 {
		t.Fatal("LinuxCIS produced an empty resource set")
	}

	ids := make([]string, len(exts))
	for i, e := range exts {
		ids[i] = e.ID()
	}
	return ids
}

func TestLinuxCIS_BuildsCleanly(t *testing.T) {
	t.Parallel()

	ids := buildCIS(t)

	// The benchmark declares a sizable set of resources across every category;
	// guard against a regression that silently drops whole groups.
	if len(ids) < 50 {
		t.Errorf("LinuxCIS resource count = %d, want >= 50", len(ids))
	}

	// Every resource ID must be unique; a collision would have surfaced as a
	// BuildGraph error, but assert it explicitly to document the expectation.
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate resource ID: %q", id)
		}
		seen[id] = true
	}
}

func TestLinuxCIS_CoversAllCategories(t *testing.T) {
	t.Parallel()

	ids := buildCIS(t)

	// One representative resource per cis* sub-function. If any sub-function is
	// dropped from LinuxCIS its marker disappears here.
	want := []string{
		"file:/etc/modprobe.d/cis-cramfs.conf", // cisFilesystems
		"sysctl:kernel.randomize_va_space",     // cisSysctl
		"service:avahi-daemon",                 // cisServices
		"package:telnet",                       // cisPackages
		"package:auditd",                       // cisPackages (ensure)
		"file:/etc/ssh/sshd_config",            // cisSSH
		"file:/etc/security/pwquality.conf",    // cisPAM
		"file:/etc/issue.net",                  // cisBanners
		"file:/etc/shadow",                     // cisPermissions
		"service:auditd",                       // cisAudit
		"file:/etc/audit/rules.d/cis.rules",    // cisAudit
		"file:/etc/login.defs",                 // cisAuth
	}

	for _, id := range want {
		if !slices.Contains(ids, id) {
			t.Errorf("LinuxCIS missing expected resource %q", id)
		}
	}
}
