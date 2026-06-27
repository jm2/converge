package pkg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TsekNet/converge/internal/testutil"
)

// errManager is a PackageManager whose IsInstalled always errors, used to
// exercise Package.Check's error-propagation branch.
type errManager struct{ name string }

func (e *errManager) Name() string { return e.name }
func (e *errManager) IsInstalled(context.Context, string) (bool, error) {
	return false, fmt.Errorf("query failed")
}
func (e *errManager) Install(context.Context, string) error { return nil }
func (e *errManager) Remove(context.Context, string) error  { return nil }

func TestPackage_Check_IsInstalledError(t *testing.T) {
	p := &Package{PkgName: "git", State: "present", Manager: &errManager{name: "mock"}}
	if _, err := p.Check(context.Background()); err == nil {
		t.Error("Check() should propagate IsInstalled error")
	}
}

func TestPackage_PollInterval(t *testing.T) {
	p := &Package{PkgName: "git"}
	if got := p.PollInterval(); got != 5*time.Minute {
		t.Errorf("PollInterval() = %v, want %v", got, 5*time.Minute)
	}
}

// TestPackage_WithMockPackageManager drives the resource through the shared
// testutil mock to cover the convergence and drift paths end-to-end.
func TestPackage_WithMockPackageManager(t *testing.T) {
	t.Run("install converges", func(t *testing.T) {
		mgr := testutil.NewMockPackageManager("apt")
		p := &Package{PkgName: "git", State: "present", Manager: mgr}
		testutil.AssertConverges(t, p)
		if !mgr.Installed["git"] {
			t.Error("package should be installed after Apply")
		}
	})

	t.Run("already present is in sync", func(t *testing.T) {
		mgr := testutil.NewMockPackageManager("apt")
		mgr.Installed["git"] = true
		p := &Package{PkgName: "git", State: "present", Manager: mgr}
		testutil.AssertInSync(t, p)
	})

	t.Run("remove converges", func(t *testing.T) {
		mgr := testutil.NewMockPackageManager("apt")
		mgr.Installed["git"] = true
		p := &Package{PkgName: "git", State: "absent", Manager: mgr}
		testutil.AssertConverges(t, p)
		if mgr.Installed["git"] {
			t.Error("package should be removed after Apply")
		}
	})

	t.Run("apply remove error", func(t *testing.T) {
		mgr := testutil.NewMockPackageManager("apt").WithRemoveError("boom")
		mgr.Installed["git"] = true
		p := &Package{PkgName: "git", State: "absent", Manager: mgr}
		if _, err := p.Apply(context.Background()); err == nil {
			t.Error("Apply() should return remove error")
		}
	})
}
