package secpol

import "fmt"

// Opts configures a SecurityPolicy resource.
type Opts struct {
	Category string
	Key      string
	Value    string
	Critical bool
}

// SecurityPolicy enforces Windows local security policy (password and lockout settings)
// via the NetUserModalsGet/Set Win32 API (no secedit.exe).
type SecurityPolicy struct {
	Category string
	Key      string
	Value    string
	Critical bool
}

func New(name string, opts Opts) *SecurityPolicy {
	return &SecurityPolicy{
		Category: opts.Category,
		Key:      opts.Key,
		Value:    opts.Value,
		Critical: opts.Critical,
	}
}

func (s *SecurityPolicy) ID() string { return fmt.Sprintf("secpol:%s:%s", s.Category, s.Key) }
func (s *SecurityPolicy) String() string {
	return fmt.Sprintf("SecurityPolicy %s/%s", s.Category, s.Key)
}
func (s *SecurityPolicy) IsCritical() bool { return s.Critical }
