//go:build windows

package dsl

func (r *Run) Registry(key string, opts RegistryOpts) {
	if !r.require("Registry", "key", key) {
		return
	}
	r.addResource(newRegistryExtension(key, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) SecurityPolicy(name string, opts SecurityPolicyOpts) {
	if !r.require("SecurityPolicy", "name", name) {
		return
	}
	if !r.require("SecurityPolicy", "category", opts.Category) {
		return
	}
	if !r.require("SecurityPolicy", "key", opts.Key) {
		return
	}
	r.addResource(newSecurityPolicyExtension(name, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) AuditPolicy(name string, opts AuditPolicyOpts) {
	if !r.require("AuditPolicy", "name", name) {
		return
	}
	if !r.require("AuditPolicy", "subcategory", opts.Subcategory) {
		return
	}
	r.addResource(newAuditPolicyExtension(name, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}
