package testutil

import (
	"context"
	"fmt"
)

// MockPackageManager implements pkg.PackageManager for testing.
// It tracks installed packages in memory and supports custom error injection.
type MockPackageManager struct {
	ManagerName string
	Installed   map[string]bool
	InstallFn   func(string) error // optional override
	RemoveFn    func(string) error // optional override
}

// NewMockPackageManager creates a MockPackageManager with no packages installed.
func NewMockPackageManager(name string) *MockPackageManager {
	return &MockPackageManager{
		ManagerName: name,
		Installed:   make(map[string]bool),
	}
}

func (m *MockPackageManager) Name() string { return m.ManagerName }

func (m *MockPackageManager) IsInstalled(_ context.Context, name string) (bool, error) {
	return m.Installed[name], nil
}

func (m *MockPackageManager) Install(_ context.Context, name string) error {
	if m.InstallFn != nil {
		return m.InstallFn(name)
	}
	m.Installed[name] = true
	return nil
}

func (m *MockPackageManager) Remove(_ context.Context, name string) error {
	if m.RemoveFn != nil {
		return m.RemoveFn(name)
	}
	delete(m.Installed, name)
	return nil
}

// WithInstallError returns a MockPackageManager whose Install always fails.
func (m *MockPackageManager) WithInstallError(msg string) *MockPackageManager {
	m.InstallFn = func(string) error { return fmt.Errorf("%s", msg) }
	return m
}

// WithRemoveError returns a MockPackageManager whose Remove always fails.
func (m *MockPackageManager) WithRemoveError(msg string) *MockPackageManager {
	m.RemoveFn = func(string) error { return fmt.Errorf("%s", msg) }
	return m
}
