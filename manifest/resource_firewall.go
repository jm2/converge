package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extfw "github.com/TsekNet/converge/extensions/firewall"
)

func init() { register("firewall", decodeFirewall) }

type firewallBlock struct {
	Port      int    `hcl:"port,optional"`
	Protocol  string `hcl:"protocol,optional"`
	Direction string `hcl:"direction,optional"`
	Action    string `hcl:"action,optional"`
	Source    string `hcl:"source,optional"`
	Dest      string `hcl:"dest,optional"`
	Ensure    string `hcl:"ensure,optional"`
	Critical  bool   `hcl:"critical,optional"`
}

func decodeFirewall(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var fb firewallBlock
	diags := gohcl.DecodeBody(body, nil, &fb)
	if diags.HasErrors() {
		return nil, diags
	}
	protocol := fb.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	direction := fb.Direction
	if direction == "" {
		direction = "inbound"
	}
	action := fb.Action
	if action == "" {
		action = "allow"
	}
	ensure := fb.Ensure
	if ensure == "" {
		ensure = "present"
	}
	return extfw.New(name, extfw.Opts{
		Port:      fb.Port,
		Protocol:  protocol,
		Direction: direction,
		Action:    action,
		Source:    fb.Source,
		Dest:      fb.Dest,
		State:     ensure,
		Critical:  fb.Critical,
	}), nil
}
