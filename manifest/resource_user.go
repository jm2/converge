package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extuser "github.com/TsekNet/converge/extensions/user"
)

func init() { register("user", decodeUser) }

type userBlock struct {
	Name     string   `hcl:"name,optional"`
	Groups   []string `hcl:"groups,optional"`
	Shell    string   `hcl:"shell,optional"`
	Home     string   `hcl:"home,optional"`
	System   bool     `hcl:"system,optional"`
	Critical bool     `hcl:"critical,optional"`
}

func decodeUser(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var ub userBlock
	diags := gohcl.DecodeBody(body, nil, &ub)
	if diags.HasErrors() {
		return nil, diags
	}
	userName := ub.Name
	if userName == "" {
		userName = name
	}
	return extuser.New(userName, extuser.Opts{
		Groups:   ub.Groups,
		Shell:    ub.Shell,
		Home:     ub.Home,
		System:   ub.System,
		Critical: ub.Critical,
	}), nil
}
