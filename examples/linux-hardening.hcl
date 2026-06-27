# linux-hardening.hcl — Linux-only resource types (sysctl, kernelmodule).
#
# These types are registered only on Linux, matching the Go DSL.
#
#   sudo converge serve ./examples/linux-hardening.hcl

resource "sysctl" "ip_forward" {
  key     = "net.ipv4.ip_forward"
  value   = "0"
  persist = true
}

resource "kernelmodule" "usb_storage" {
  module = "usb-storage"
  ensure = "blacklisted"
}

resource "file" "login_banner" {
  path    = "/etc/issue.net"
  content = "Authorized access only.\n"
  mode    = "0644"
}
