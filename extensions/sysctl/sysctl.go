package sysctl

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/TsekNet/converge/extensions"
)

var validKeyRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// Sysctl manages a Linux kernel parameter. Check reads from /proc/sys/,
// Apply writes the value and optionally persists it to /etc/sysctl.d/.
type Sysctl struct {
	Key      string
	Value    string
	Persist  bool
	Critical bool
	FS       extensions.FS // nil uses the real OS filesystem
}

// Opts holds configurable fields for a Sysctl resource.
type Opts struct {
	Value    string
	Persist  bool
	Critical bool
	FS       extensions.FS // inject a mock for testing
}

func New(key string, opts Opts) *Sysctl {
	return &Sysctl{
		Key:      key,
		Value:    opts.Value,
		Persist:  opts.Persist,
		Critical: opts.Critical,
		FS:       opts.FS,
	}
}

func (s *Sysctl) fsys() extensions.FS { return extensions.RealFS(s.FS) }

// validate checks that Key contains only safe characters and no path traversal.
func (s *Sysctl) validate() error {
	if !validKeyRe.MatchString(s.Key) {
		return fmt.Errorf("sysctl key %q contains invalid characters", s.Key)
	}
	if strings.Contains(s.Key, "..") {
		return fmt.Errorf("sysctl key %q contains path traversal", s.Key)
	}
	return nil
}

func (s *Sysctl) ID() string       { return fmt.Sprintf("sysctl:%s", s.Key) }
func (s *Sysctl) String() string   { return fmt.Sprintf("Sysctl %s = %s", s.Key, s.Value) }
func (s *Sysctl) IsCritical() bool { return s.Critical }
