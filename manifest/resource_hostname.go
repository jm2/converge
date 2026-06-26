package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	exthostname "github.com/TsekNet/converge/extensions/hostname"
)

func init() { register("hostname", decodeHostname) }

type hostnameBlock struct {
	Name     string `hcl:"name,optional"`
	Critical bool   `hcl:"critical,optional"`
}

func decodeHostname(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var hb hostnameBlock
	diags := gohcl.DecodeBody(body, nil, &hb)
	if diags.HasErrors() {
		return nil, diags
	}
	hostName := hb.Name
	if hostName == "" {
		hostName = name
	}
	return exthostname.New(hostName, exthostname.Opts{
		Critical: hb.Critical,
	}), nil
}
