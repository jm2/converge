//go:build linux

package manifest

import "testing"

// linux-hardening.hcl uses sysctl/kernelmodule, which register only on Linux.
func TestExampleLinuxHardening(t *testing.T) { loadExample(t, "linux-hardening.hcl") }
