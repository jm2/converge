//go:build linux

package dsl

func (r *Run) Sysctl(key string, opts SysctlOpts) {
	if !r.require("Sysctl", "key", key) {
		return
	}
	if !r.require("Sysctl", "value", opts.Value) {
		return
	}
	r.addResource(newSysctlExtension(key, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}

func (r *Run) KernelModule(module string, opts KernelModuleOpts) {
	if !r.require("KernelModule", "module", module) {
		return
	}
	if opts.State == "" {
		opts.State = ModuleLoaded
	}
	r.addResource(newKernelModuleExtension(module, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}
