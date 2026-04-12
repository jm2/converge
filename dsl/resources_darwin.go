//go:build darwin

package dsl

import (
	"github.com/TsekNet/converge/extensions"
	extplist "github.com/TsekNet/converge/extensions/plist"
)

func newPlistExtension(domain string, opts PlistOpts) extensions.Extension {
	return extplist.New(domain, extplist.Opts{
		Key:      opts.Key,
		Value:    opts.Value,
		Type:     opts.Type,
		Host:     opts.Host,
		Critical: opts.Critical,
	})
}
