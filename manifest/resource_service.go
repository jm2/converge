package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extsvc "github.com/TsekNet/converge/extensions/service"
)

func init() { register("service", decodeService) }

// serviceBlock mirrors extensions/service.Opts. `ensure` is the run state
// ("running"/"stopped"); InitSystem is threaded from the detected platform, not
// authored, matching dsl.Run.Service.
type serviceBlock struct {
	Name        string `hcl:"name,optional"`
	Ensure      string `hcl:"ensure,optional"`       // "running" or "stopped" (default running)
	Enable      bool   `hcl:"enable,optional"`       // start on boot
	StartupType string `hcl:"startup_type,optional"` // Windows SCM: auto/delayed-auto/manual/disabled
	Critical    bool   `hcl:"critical,optional"`
}

func decodeService(name string, body hcl.Body, dctx *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var sb serviceBlock
	diags := gohcl.DecodeBody(body, nil, &sb)
	if diags.HasErrors() {
		return nil, diags
	}

	svcName := sb.Name
	if svcName == "" {
		svcName = name
	}

	ensure := sb.Ensure
	if ensure == "" {
		ensure = "running" // default matches dsl.Run.Service
	}

	return extsvc.New(svcName, extsvc.Opts{
		State:       ensure,
		Enable:      sb.Enable,
		StartupType: sb.StartupType,
		InitSystem:  dctx.platform.InitSystem,
		Critical:    sb.Critical,
	}), nil
}
