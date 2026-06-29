package service

import "fmt"

// Opts configures a Service resource.
type Opts struct {
	State       string // "running" or "stopped"
	Enable      bool
	StartupType string // "auto", "delayed-auto", "manual", "disabled" (Windows SCM)
	InitSystem  string
	Critical    bool
}

// Service manages a system service. Check/Apply are in platform-specific files
// (systemd on Linux, SCM on Windows, launchd on macOS).
type Service struct {
	Name        string
	State       string // "running" or "stopped"
	Enable      bool
	StartupType string // "auto", "delayed-auto", "manual", "disabled" (Windows SCM)
	InitSystem  string
	Critical    bool
}

func New(name string, opts Opts) *Service {
	return &Service{
		Name:        name,
		State:       opts.State,
		Enable:      opts.Enable,
		StartupType: opts.StartupType,
		InitSystem:  opts.InitSystem,
		Critical:    opts.Critical,
	}
}

func (s *Service) ID() string       { return fmt.Sprintf("service:%s", s.Name) }
func (s *Service) String() string   { return fmt.Sprintf("Service %s", s.Name) }
func (s *Service) IsCritical() bool { return s.Critical }
