package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extfile "github.com/TsekNet/converge/extensions/file"
)

func init() { register("file", decodeFile) }

// fileBlock mirrors extensions/file.Opts. `ensure` maps to the string State the
// extension expects ("present"/"absent"); it is only meaningful in block mode.
type fileBlock struct {
	Path         string `hcl:"path,optional"`
	Content      string `hcl:"content,optional"`
	Mode         string `hcl:"mode,optional"` // octal string, e.g. "0644"
	Owner        string `hcl:"owner,optional"`
	Group        string `hcl:"group,optional"`
	Append       bool   `hcl:"append,optional"`
	URL          string `hcl:"url,optional"`
	Checksum     string `hcl:"checksum,optional"`
	BlockName    string `hcl:"block_name,optional"`
	BlockComment string `hcl:"block_comment,optional"`
	Ensure       string `hcl:"ensure,optional"`
	Critical     bool   `hcl:"critical,optional"`
}

func decodeFile(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var fb fileBlock
	diags := gohcl.DecodeBody(body, nil, &fb)
	if diags.HasErrors() {
		return nil, diags
	}

	path := fb.Path
	if path == "" {
		path = name // default the managed path to the block name
	}

	opts := extfile.Opts{
		Content:      fb.Content,
		Owner:        fb.Owner,
		Group:        fb.Group,
		Append:       fb.Append,
		URL:          fb.URL,
		Checksum:     fb.Checksum,
		BlockName:    fb.BlockName,
		BlockComment: fb.BlockComment,
		State:        fb.Ensure,
		Critical:     fb.Critical,
	}
	if fb.Mode != "" {
		mode, err := parseFileMode(fb.Mode)
		if err != nil {
			return nil, hcl.Diagnostics{errDiag("invalid file mode", err.Error(), body.MissingItemRange().Ptr())}
		}
		opts.Mode = mode
	}
	return extfile.New(path, opts), nil
}
