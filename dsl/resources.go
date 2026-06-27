package dsl

import (
	"github.com/TsekNet/converge/extensions"
	extcron "github.com/TsekNet/converge/extensions/cron"
	extexec "github.com/TsekNet/converge/extensions/exec"
	extfile "github.com/TsekNet/converge/extensions/file"
	extfw "github.com/TsekNet/converge/extensions/firewall"
	exthostname "github.com/TsekNet/converge/extensions/hostname"
	extpkg "github.com/TsekNet/converge/extensions/pkg"
	extreboot "github.com/TsekNet/converge/extensions/reboot"
	extsvc "github.com/TsekNet/converge/extensions/service"
	exttmpl "github.com/TsekNet/converge/extensions/template"
	extuser "github.com/TsekNet/converge/extensions/user"
)

func newFileExtension(path string, opts FileOpts) extensions.Extension {
	state := ""
	if opts.State == Absent {
		state = "absent"
	} else if opts.State == Present {
		state = "present"
	}
	return extfile.New(path, extfile.Opts{
		Content:      opts.Content,
		Mode:         opts.Mode,
		Owner:        opts.Owner,
		Group:        opts.Group,
		Append:       opts.Append,
		URL:          opts.URL,
		Checksum:     opts.Checksum,
		BlockName:    opts.BlockName,
		BlockComment: opts.BlockComment,
		State:        state,
		Critical:     opts.Critical,
		Sensitive:    opts.Sensitive,
	})
}

func newPackageExtension(name string, opts PackageOpts, pkgManager string) extensions.Extension {
	return extpkg.New(name, extpkg.Opts{
		State:       string(opts.State),
		ManagerName: pkgManager,
		Critical:    opts.Critical,
	})
}

func newServiceExtension(name string, opts ServiceOpts, initSystem string) extensions.Extension {
	return extsvc.New(name, extsvc.Opts{
		State:       string(opts.State),
		Enable:      opts.Enable,
		StartupType: opts.StartupType,
		InitSystem:  initSystem,
		Critical:    opts.Critical,
	})
}

func newExecExtension(name string, opts ExecOpts) extensions.Extension {
	return extexec.New(name, extexec.Opts{
		Command:     opts.Command,
		Args:        opts.Args,
		Shell:       opts.Shell,
		ShellParams: opts.ShellParams,
		Dir:         opts.Dir,
		Env:         opts.Env,
		Retries:     opts.Retries,
		RetryDelay:  opts.RetryDelay,
		Critical:    opts.Critical,
		Creates:     opts.Creates,
		OnlyIf:      opts.OnlyIf,
		Unless:      opts.Unless,
	})
}

func newUserExtension(name string, opts UserOpts) extensions.Extension {
	return extuser.New(name, extuser.Opts{
		Groups:   opts.Groups,
		Shell:    opts.Shell,
		Home:     opts.Home,
		System:   opts.System,
		Critical: opts.Critical,
	})
}

func newRebootExtension(name string, opts RebootOpts) extensions.Extension {
	return extreboot.New(name, extreboot.Opts{
		Reason:   opts.Reason,
		Message:  opts.Message,
		Delay:    opts.Delay,
		Critical: opts.Critical,
	})
}

func newFirewallExtension(name string, opts FirewallOpts) extensions.Extension {
	state := "present"
	if opts.State == Absent {
		state = "absent"
	}
	return extfw.New(name, extfw.Opts{
		Port:      opts.Port,
		Protocol:  opts.Protocol,
		Direction: opts.Direction,
		Action:    opts.Action,
		Source:    opts.Source,
		Dest:      opts.Dest,
		State:     state,
		Critical:  opts.Critical,
	})
}

func newTemplateExtension(path string, opts TemplateOpts) extensions.Extension {
	return exttmpl.New(path, exttmpl.Opts{
		Source:   opts.Source,
		Vars:     opts.Vars,
		Mode:     opts.Mode,
		Owner:    opts.Owner,
		Group:    opts.Group,
		Critical: opts.Critical,
	})
}

func newHostnameExtension(name string, opts HostnameOpts) extensions.Extension {
	return exthostname.New(name, exthostname.Opts{
		Critical: opts.Critical,
	})
}

func newCronExtension(name string, opts CronOpts) extensions.Extension {
	state := "present"
	if opts.State == Absent {
		state = "absent"
	}
	return extcron.New(name, extcron.Opts{
		Schedule: opts.Schedule,
		Command:  opts.Command,
		User:     opts.User,
		State:    state,
		Critical: opts.Critical,
	})
}
