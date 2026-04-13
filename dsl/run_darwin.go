//go:build darwin

package dsl

func (r *Run) Plist(domain string, opts PlistOpts) {
	if !r.require("Plist", "domain", domain) {
		return
	}
	if !r.require("Plist", "key", opts.Key) {
		return
	}
	r.addResource(newPlistExtension(domain, opts), nm(opts.Noop, opts.Retry, opts.Limit, opts.AutoEdge, opts.AutoGroup, opts.Condition))
}
