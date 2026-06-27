package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extcron "github.com/TsekNet/converge/extensions/cron"
)

func init() { register("cron", decodeCron) }

type cronBlock struct {
	Schedule string `hcl:"schedule"`
	Command  string `hcl:"command"`
	User     string `hcl:"user,optional"`
	Ensure   string `hcl:"ensure,optional"`
	Critical bool   `hcl:"critical,optional"`
}

func decodeCron(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var cb cronBlock
	diags := gohcl.DecodeBody(body, nil, &cb)
	if diags.HasErrors() {
		return nil, diags
	}
	ensure := cb.Ensure
	if ensure == "" {
		ensure = "present"
	}
	return extcron.New(name, extcron.Opts{
		Schedule: cb.Schedule,
		Command:  cb.Command,
		User:     cb.User,
		State:    ensure,
		Critical: cb.Critical,
	}), nil
}
