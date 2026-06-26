package manifest

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	"github.com/TsekNet/converge/extensions"
	extpkg "github.com/TsekNet/converge/extensions/pkg"
)

func init() {
	register("package", decodePackage)
	register("repository", decodeRepository)
}

// packageBlock mirrors extensions/pkg.Opts. ManagerName is threaded from the
// detected platform; `ensure` defaults to "present" (matching dsl.Run.Package).
type packageBlock struct {
	Name     string `hcl:"name,optional"`
	Ensure   string `hcl:"ensure,optional"` // "present" or "absent" (default present)
	Critical bool   `hcl:"critical,optional"`
}

func decodePackage(name string, body hcl.Body, dctx *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var pb packageBlock
	diags := gohcl.DecodeBody(body, nil, &pb)
	if diags.HasErrors() {
		return nil, diags
	}

	pkgName := pb.Name
	if pkgName == "" {
		pkgName = name
	}
	ensure := pb.Ensure
	if ensure == "" {
		ensure = "present"
	}

	return extpkg.New(pkgName, extpkg.Opts{
		State:       ensure,
		ManagerName: dctx.platform.PkgManager,
		Critical:    pb.Critical,
	}), nil
}

// repositoryBlock mirrors extensions/pkg.RepositoryOpts. uri is required;
// ManagerName is threaded from the detected platform.
type repositoryBlock struct {
	Name         string `hcl:"name,optional"`
	URI          string `hcl:"uri"`
	Distribution string `hcl:"distribution,optional"`
	Components   string `hcl:"components,optional"`
	GPGKey       string `hcl:"gpg_key,optional"`
	Enabled      bool   `hcl:"enabled,optional"`
	Ensure       string `hcl:"ensure,optional"` // "present" or "absent" (default present)
	Critical     bool   `hcl:"critical,optional"`
}

func decodeRepository(name string, body hcl.Body, dctx *decodeContext) (extensions.Extension, hcl.Diagnostics) {
	var rb repositoryBlock
	diags := gohcl.DecodeBody(body, nil, &rb)
	if diags.HasErrors() {
		return nil, diags
	}

	repoName := rb.Name
	if repoName == "" {
		repoName = name
	}
	ensure := rb.Ensure
	if ensure == "" {
		ensure = "present"
	}

	return extpkg.NewRepository(repoName, extpkg.RepositoryOpts{
		URI:          rb.URI,
		Distribution: rb.Distribution,
		Components:   rb.Components,
		GPGKey:       rb.GPGKey,
		Enabled:      rb.Enabled,
		State:        ensure,
		ManagerName:  dctx.platform.PkgManager,
		Critical:     rb.Critical,
	}), nil
}
