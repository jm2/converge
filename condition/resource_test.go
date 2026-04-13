package condition

import (
	"context"
	"testing"
)

func TestTypedResourceConstructors(t *testing.T) {
	tests := []struct {
		name   string
		cond   *resourceCondition
		wantID string
	}{
		{"File", File("/etc/config"), "file:/etc/config"},
		{"Package", Package("nginx"), "package:nginx"},
		{"Service", Service("nginx"), "service:nginx"},
		{"Exec", Exec("install"), "exec:install"},
		{"User", User("deploy"), "user:deploy"},
		{"Template", Template("/etc/nginx.conf"), "template:/etc/nginx.conf"},
		{"Hostname", Hostname("web01"), "hostname:web01"},
		{"Cron", Cron("backup"), "cron:backup"},
		{"Repository", Repository("chrome"), "repository:chrome"},
		{"Firewall", Firewall("Allow SSH"), "firewall:Allow SSH"},
		{"Reboot", Reboot("driver-install"), "reboot:driver-install"},
		{"Sysctl", Sysctl("net.ipv4.ip_forward"), "sysctl:net.ipv4.ip_forward"},
		{"KernelModule", KernelModule("cramfs"), "kernelmodule:cramfs"},
		{"Registry", Registry(`HKLM\SOFTWARE\Test`), `registry:HKLM\SOFTWARE\Test`},
		{"Plist", Plist("com.apple.SoftwareUpdate"), "plist:com.apple.SoftwareUpdate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cond.ID() != tt.wantID {
				t.Errorf("ID() = %q, want %q", tt.cond.ID(), tt.wantID)
			}
		})
	}
}

func TestResourceCondition_AlwaysMet(t *testing.T) {
	ctx := context.Background()
	c := Package("nginx")
	met, err := c.Met(ctx)
	if err != nil {
		t.Fatalf("Met() error = %v", err)
	}
	if !met {
		t.Error("resource condition should always be met at runtime")
	}
}

func TestResource_EscapeHatch(t *testing.T) {
	c := Resource("custom:something")
	if c.ID() != "custom:something" {
		t.Errorf("ID() = %q, want %q", c.ID(), "custom:something")
	}
}
