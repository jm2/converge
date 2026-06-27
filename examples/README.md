# Example HCL manifests

Runnable [HCL manifests](../docs/hcl.md) for converge's HCL front-end. They are
the HCL counterpart to the Go blueprints in [`blueprints/`](../blueprints/).

```
converge plan  ./examples/baseline.hcl
sudo converge serve ./examples/web-stack.hcl
```

| File | Shows |
|------|-------|
| [baseline.hcl](baseline.hcl) | A minimal cross-platform baseline: package, file, service, firewall |
| [web-stack.hcl](web-stack.hcl) | Dependency ordering with `require`/`notify` |
| [linux-hardening.hcl](linux-hardening.hcl) | Linux-only resources (`sysctl`, `kernelmodule`) |

These manifests are validated against the parser in
[`manifest/examples_test.go`](../manifest/examples_test.go), so they stay
accurate to the implementation.
