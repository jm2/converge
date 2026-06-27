package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extreboot "github.com/TsekNet/converge/extensions/reboot"
)

func init() { register("reboot", decodeReboot) }

type rebootBlock struct {
	Name     string `hcl:"name,optional"`
	Reason   string `hcl:"reason,optional"`
	Message  string `hcl:"message,optional"`
	Delay    string `hcl:"delay,optional"`
	Critical bool   `hcl:"critical,optional"`
}

func decodeReboot(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var rb rebootBlock
	diags := gohcl.DecodeBody(body, nil, &rb)
	if diags.HasErrors() {
		return nil, diags
	}
	rebootName := rb.Name
	if rebootName == "" {
		rebootName = name
	}
	opts := extreboot.Opts{
		Reason:   rb.Reason,
		Message:  rb.Message,
		Critical: rb.Critical,
	}
	if rb.Delay != "" {
		delay, err := parseDuration(rb.Delay)
		if err != nil {
			return nil, hcl.Diagnostics{errDiag("invalid reboot delay", err.Error(), body.MissingItemRange().Ptr())}
		}
		opts.Delay = delay
	}
	return extreboot.New(rebootName, opts), nil
}
