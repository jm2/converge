//go:build linux

package manifest

import "testing"

// sysctl and kernelmodule are registered only on Linux (matching the Go DSL),
// so their tests are build-tagged.

func TestSysctlLoads(t *testing.T) {
	g := mustLoad(t, `
resource "sysctl" "forwarding" {
  key   = "net.ipv4.ip_forward"
  value = "1"
}
`)
	g.requireNode("sysctl:net.ipv4.ip_forward")
}

func TestSysctlMissingValueRejected(t *testing.T) {
	mustFailLoad(t, `resource "sysctl" "x" { key = "net.ipv4.ip_forward" }`)
}

func TestKernelModuleLoads(t *testing.T) {
	g := mustLoad(t, `
resource "kernelmodule" "overlay" {
  ensure = "loaded"
}
`)
	g.requireNode("kernelmodule:overlay")
}

func TestKernelModuleInvalidEnsureRejected(t *testing.T) {
	mustFailLoad(t, `resource "kernelmodule" "x" { ensure = "frobnicated" }`)
}

func TestRegistryHasLinuxTypes(t *testing.T) {
	for _, typ := range []string{"sysctl", "kernelmodule"} {
		if _, ok := registryFor(typ); !ok {
			t.Errorf("linux resource type %q is not registered", typ)
		}
	}
}
