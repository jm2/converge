//go:build linux

package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extkmod "github.com/TsekNet/converge/extensions/kernelmodule"
)

func init() { register("kernelmodule", decodeKernelModule) }

type kernelmoduleBlock struct {
	Module   string `hcl:"module,optional"`
	Ensure   string `hcl:"ensure,optional"`
	Critical bool   `hcl:"critical,optional"`
}

func decodeKernelModule(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var kb kernelmoduleBlock
	diags := gohcl.DecodeBody(body, nil, &kb)
	if diags.HasErrors() {
		return nil, diags
	}
	module := kb.Module
	if module == "" {
		module = name
	}
	ensure := kb.Ensure
	if ensure == "" {
		ensure = string(extkmod.Loaded)
	}
	var state extkmod.StateType
	switch ensure {
	case string(extkmod.Loaded):
		state = extkmod.Loaded
	case string(extkmod.Blacklisted):
		state = extkmod.Blacklisted
	default:
		return nil, hcl.Diagnostics{errDiag("invalid kernel module ensure", "ensure must be \"loaded\" or \"blacklisted\", got "+ensure, body.MissingItemRange().Ptr())}
	}
	return extkmod.New(module, extkmod.Opts{
		State:    state,
		Critical: kb.Critical,
	}), nil
}
