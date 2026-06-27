package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extexec "github.com/TsekNet/converge/extensions/exec"
)

func init() { register("exec", decodeExec) }

type execBlock struct {
	Command     string   `hcl:"command"`
	Args        []string `hcl:"args,optional"`
	Shell       string   `hcl:"shell,optional"`
	ShellParams []string `hcl:"shell_params,optional"`
	Dir         string   `hcl:"dir,optional"`
	Env         []string `hcl:"env,optional"`
	Retries     int      `hcl:"retries,optional"`
	RetryDelay  string   `hcl:"retry_delay,optional"`
	Critical    bool     `hcl:"critical,optional"`
}

func decodeExec(name string, body hcl.Body, _ *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var eb execBlock
	diags := gohcl.DecodeBody(body, nil, &eb)
	if diags.HasErrors() {
		return nil, diags
	}
	opts := extexec.Opts{
		Command:     eb.Command,
		Args:        eb.Args,
		Shell:       eb.Shell,
		ShellParams: eb.ShellParams,
		Dir:         eb.Dir,
		Env:         eb.Env,
		Retries:     eb.Retries,
		Critical:    eb.Critical,
	}
	if eb.RetryDelay != "" {
		delay, err := parseDuration(eb.RetryDelay)
		if err != nil {
			return nil, hcl.Diagnostics{errDiag("invalid retry_delay", err.Error(), body.MissingItemRange().Ptr())}
		}
		opts.RetryDelay = delay
	}
	return extexec.New(name, opts), nil
}
