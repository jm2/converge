//go:build linux

package dsl

import (
	"github.com/TsekNet/converge/extensions"
	extkmod "github.com/TsekNet/converge/extensions/kernelmodule"
	extsysctl "github.com/TsekNet/converge/extensions/sysctl"
)

func newSysctlExtension(key string, opts SysctlOpts) extensions.Extension {
	return extsysctl.New(key, extsysctl.Opts{
		Value:    opts.Value,
		Persist:  opts.Persist,
		Critical: opts.Critical,
	})
}

func newKernelModuleExtension(module string, opts KernelModuleOpts) extensions.Extension {
	return extkmod.New(module, extkmod.Opts{
		State:    extkmod.StateType(opts.State),
		Critical: opts.Critical,
	})
}
