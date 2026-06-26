# web-stack.hcl — dependency ordering with require/notify.
#
# Mirrors the "Dependencies and Conditions" example in docs/examples.md, building
# the DAG:  package:nginx -> file:/etc/nginx/conf.d/app.conf -> service:nginx
#
# In daemon mode, if the config file drifts, converge restores it and walks the
# DAG forward to re-check the service.

resource "package" "nginx" {
  ensure = "present"
}

resource "file" "app_conf" {
  path    = "/etc/nginx/conf.d/app.conf"
  content = "server { listen 80; server_name app.example.com; }\n"
  mode    = "0644"
  require = [package.nginx] # write the config after nginx is installed
  notify  = [service.nginx] # re-check nginx when the config changes
}

resource "service" "nginx" {
  ensure = "running"
  enable = true
}
