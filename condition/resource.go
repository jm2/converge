package condition

import (
	"context"
	"fmt"
)

// resourceCondition represents a DAG dependency on another resource.
// The DSL extracts these at graph-build time and converts them to edges.
// At runtime, Met always returns true (the engine enforces ordering).
type resourceCondition struct {
	id string
}

func (c *resourceCondition) ID() string                        { return c.id }
func (c *resourceCondition) Met(_ context.Context) (bool, error) { return true, nil }
func (c *resourceCondition) Wait(_ context.Context) error        { return nil }
func (c *resourceCondition) String() string                      { return fmt.Sprintf("resource(%s)", c.id) }

// Resource returns a condition that depends on a resource by its full ID
// (e.g. "file:/etc/config"). Prefer the typed constructors below.
func Resource(id string) *resourceCondition {
	return &resourceCondition{id: id}
}

// Typed resource constructors. Each builds the correct ID prefix so the
// caller passes the same name they used in the DSL method.

func File(path string) *resourceCondition       { return &resourceCondition{id: "file:" + path} }
func Package(name string) *resourceCondition     { return &resourceCondition{id: "package:" + name} }
func Service(name string) *resourceCondition     { return &resourceCondition{id: "service:" + name} }
func Exec(name string) *resourceCondition        { return &resourceCondition{id: "exec:" + name} }
func User(name string) *resourceCondition        { return &resourceCondition{id: "user:" + name} }
func Template(path string) *resourceCondition    { return &resourceCondition{id: "template:" + path} }
func Hostname(name string) *resourceCondition    { return &resourceCondition{id: "hostname:" + name} }
func Cron(name string) *resourceCondition        { return &resourceCondition{id: "cron:" + name} }
func Repository(name string) *resourceCondition  { return &resourceCondition{id: "repository:" + name} }
func Firewall(name string) *resourceCondition    { return &resourceCondition{id: "firewall:" + name} }
func Reboot(name string) *resourceCondition      { return &resourceCondition{id: "reboot:" + name} }
func Sysctl(key string) *resourceCondition       { return &resourceCondition{id: "sysctl:" + key} }
func KernelModule(name string) *resourceCondition { return &resourceCondition{id: "kernelmodule:" + name} }
func Registry(key string) *resourceCondition     { return &resourceCondition{id: "registry:" + key} }
func Plist(domain string) *resourceCondition     { return &resourceCondition{id: "plist:" + domain} }
