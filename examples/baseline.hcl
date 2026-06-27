# baseline.hcl — a minimal cross-platform baseline.
#
# Mirrors the Baseline Go blueprint in docs/examples.md: a package, a system
# banner file, a service, and an inbound SSH rule.
#
#   converge plan  ./examples/baseline.hcl
#   sudo converge serve ./examples/baseline.hcl

resource "package" "git" {
  ensure = "present"
}

resource "file" "motd" {
  path    = "/etc/motd"
  content = "Managed by Converge\n"
  mode    = "0644"
}

resource "service" "sshd" {
  ensure = "running"
  enable = true
}

resource "firewall" "allow_ssh" {
  port   = 22
  action = "allow"
}
