//go:build linux

package platform

import (
	"os/exec"
	"testing"
)

// detectDarwinPkgManager and detectWindowsPkgManager are not build-tagged, so
// they compile and run on Linux. They make their decision purely from
// exec.LookPath, so on any host we can compute the deterministic expected value
// the same way the production code does and assert the function agrees. On a
// typical Linux CI host none of brew/winget/choco are present, so the expected
// result is the empty string.

func TestDetectDarwinPkgManager_Linux(t *testing.T) {
	t.Parallel()

	want := ""
	if _, err := exec.LookPath("brew"); err == nil {
		want = "brew"
	}

	if got := detectDarwinPkgManager(); got != want {
		t.Errorf("detectDarwinPkgManager() = %q, want %q", got, want)
	}
}

func TestDetectWindowsPkgManager_Linux(t *testing.T) {
	t.Parallel()

	want := ""
	if _, err := exec.LookPath("winget"); err == nil {
		want = "winget"
	} else if _, err := exec.LookPath("choco"); err == nil {
		want = "choco"
	}

	if got := detectWindowsPkgManager(); got != want {
		t.Errorf("detectWindowsPkgManager() = %q, want %q", got, want)
	}
}
