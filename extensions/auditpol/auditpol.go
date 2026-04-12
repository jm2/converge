package auditpol

import "fmt"

// Opts configures an AuditPolicy resource.
type Opts struct {
	Subcategory string
	Success     bool
	Failure     bool
	Critical    bool
}

// AuditPolicy configures Windows advanced audit policy subcategories
// via AuditQuerySystemPolicy/AuditSetSystemPolicy (no auditpol.exe).
type AuditPolicy struct {
	Subcategory string
	Success     bool
	Failure     bool
	Critical    bool
}

func New(name string, opts Opts) *AuditPolicy {
	return &AuditPolicy{
		Subcategory: opts.Subcategory,
		Success:     opts.Success,
		Failure:     opts.Failure,
		Critical:    opts.Critical,
	}
}

func (a *AuditPolicy) ID() string       { return fmt.Sprintf("auditpol:%s", a.Subcategory) }
func (a *AuditPolicy) String() string   { return fmt.Sprintf("AuditPolicy %s", a.Subcategory) }
func (a *AuditPolicy) IsCritical() bool { return a.Critical }
