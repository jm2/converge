package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	exttmpl "github.com/TsekNet/converge/extensions/template"
)

func init() { register("template", decodeTemplate) }

type templateBlock struct {
	Path     string            `hcl:"path,optional"`
	Source   string            `hcl:"source"`
	Vars     map[string]string `hcl:"vars,optional"`
	Mode     string            `hcl:"mode,optional"`
	Owner    string            `hcl:"owner,optional"`
	Group    string            `hcl:"group,optional"`
	Critical bool              `hcl:"critical,optional"`
}

func decodeTemplate(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var tb templateBlock
	diags := gohcl.DecodeBody(body, nil, &tb)
	if diags.HasErrors() {
		return nil, diags
	}
	path := tb.Path
	if path == "" {
		path = name
	}
	opts := exttmpl.Opts{
		Source:   tb.Source,
		Vars:     tb.Vars,
		Owner:    tb.Owner,
		Group:    tb.Group,
		Critical: tb.Critical,
	}
	if tb.Mode != "" {
		mode, err := parseFileMode(tb.Mode)
		if err != nil {
			return nil, hcl.Diagnostics{errDiag("invalid file mode", err.Error(), body.MissingItemRange().Ptr())}
		}
		opts.Mode = mode
	}
	return exttmpl.New(path, opts), nil
}
