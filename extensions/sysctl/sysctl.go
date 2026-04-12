package sysctl

import "fmt"

// Sysctl manages a Linux kernel parameter. Check reads from /proc/sys/,
// Apply writes the value and optionally persists it to /etc/sysctl.d/.
type Sysctl struct {
	Key      string
	Value    string
	Persist  bool
	Critical bool
}

// Opts holds configurable fields for a Sysctl resource.
type Opts struct {
	Value    string
	Persist  bool
	Critical bool
}

func New(key string, opts Opts) *Sysctl {
	return &Sysctl{
		Key:      key,
		Value:    opts.Value,
		Persist:  opts.Persist,
		Critical: opts.Critical,
	}
}

func (s *Sysctl) ID() string       { return fmt.Sprintf("sysctl:%s", s.Key) }
func (s *Sysctl) String() string   { return fmt.Sprintf("Sysctl %s = %s", s.Key, s.Value) }
func (s *Sysctl) IsCritical() bool { return s.Critical }
