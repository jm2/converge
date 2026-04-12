//go:build windows

package dsl

import (
	"github.com/TsekNet/converge/extensions"
	extaudit "github.com/TsekNet/converge/extensions/auditpol"
	extreg "github.com/TsekNet/converge/extensions/registry"
	extsecpol "github.com/TsekNet/converge/extensions/secpol"
)

func newRegistryExtension(key string, opts RegistryOpts) extensions.Extension {
	state := "present"
	if opts.State == Absent {
		state = "absent"
	}
	return extreg.New(key, extreg.Opts{
		Value:    opts.Value,
		Type:     opts.Type,
		Data:     opts.Data,
		State:    state,
		Critical: opts.Critical,
	})
}

func newSecurityPolicyExtension(_ string, opts SecurityPolicyOpts) extensions.Extension {
	return extsecpol.New("", extsecpol.Opts{
		Category: opts.Category,
		Key:      opts.Key,
		Value:    opts.Value,
		Critical: opts.Critical,
	})
}

func newAuditPolicyExtension(_ string, opts AuditPolicyOpts) extensions.Extension {
	return extaudit.New("", extaudit.Opts{
		Subcategory: opts.Subcategory,
		Success:     opts.Success,
		Failure:     opts.Failure,
		Critical:    opts.Critical,
	})
}
