//go:build linux

package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extsysctl "github.com/TsekNet/converge/extensions/sysctl"
)

func init() { register("sysctl", decodeSysctl) }

type sysctlBlock struct {
	Key      string `hcl:"key,optional"`
	Value    string `hcl:"value"`
	Persist  bool   `hcl:"persist,optional"`
	Critical bool   `hcl:"critical,optional"`
}

func decodeSysctl(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var sb sysctlBlock
	diags := gohcl.DecodeBody(body, nil, &sb)
	if diags.HasErrors() {
		return nil, diags
	}
	key := sb.Key
	if key == "" {
		key = name
	}
	return extsysctl.New(key, extsysctl.Opts{
		Value:    sb.Value,
		Persist:  sb.Persist,
		Critical: sb.Critical,
	}), nil
}
