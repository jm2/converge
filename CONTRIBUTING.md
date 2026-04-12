# Contributing to Converge

## Development Setup

```bash
git clone https://github.com/TsekNet/converge.git && cd converge
go build -o bin/converge ./cmd/converge
go test -race ./...
```

Requires Go 1.26+. No other dependencies.

## Project Layout

| Directory | Visibility | Purpose |
|-----------|-----------|---------|
| [`dsl/`](dsl/) | Public | SDK for blueprint authors (Run, opts, enums). Platform-specific methods in build-tagged files. |
| [`extensions/`](extensions/) | Public | OS interaction: one subdirectory per resource type (see directory for full list) |
| [`blueprints/`](blueprints/) | Public | Built-in blueprints including [CIS benchmarks](blueprints/cis/) |
| [`internal/`](internal/) | Private | [Engine](internal/engine/), [output](internal/output/), [platform detection](internal/platform/), [logging](internal/logging/) |
| [`cmd/converge/`](cmd/converge/) | Binary | Cobra CLI entry point with build-tagged blueprint registration |

## Adding an Extension

See **[docs/extensions.md](docs/extensions.md)** for the full guide including platform-specific build tags.

Short version: implement the [`Extension` interface](extensions/extension.go), drop a file in the right `extensions/` subdirectory, add tests, open a PR.

## Code Standards

- **Go 1.26** -- use `slices`, `maps`, range-over-int
- **Table-driven tests** everywhere
- **Build tags** -- use `_linux`, `_darwin`, `_windows` (not `_unix` or `!windows`)
- **No stubs** -- platform-specific DSL methods live in build-tagged files; if a platform doesn't need an extension, the DSL doesn't expose it
- **Native APIs** -- prefer `golang.org/x/sys/windows`, `/proc/sys/`, `howett.net/plist` over shelling out
- **Error wrapping** with `fmt.Errorf("...: %w", err)`
- **Logging** via [google/deck](https://github.com/google/deck) -- syslog on Linux, Event Log on Windows
- **Builds** via [GoReleaser](https://goreleaser.com/) -- see [.goreleaser.yml](.goreleaser.yml)

## Testing Helpers (`internal/testutil`)

The [`internal/testutil`](internal/testutil/) package provides shared test infrastructure. Extensions should use these instead of rolling their own mocks.

| Helper | Purpose | Example |
|---|---|---|
| `MapFS` | In-memory `extensions.FS` for testing Check/Apply without root or real filesystem | `mfs := testutil.NewMapFS(); mfs.Set("/etc/conf", data, 0644)` |
| `AssertConverges(t, ext)` | Verifies the Check/Apply/Check contract: not-in-sync, Apply succeeds, in-sync | `testutil.AssertConverges(t, myFile)` |
| `AssertInSync(t, ext)` | Verifies Check reports in-sync | `testutil.AssertInSync(t, myFile)` |
| `AssertDrifted(t, ext)` | Verifies Check reports drift with changes | `testutil.AssertDrifted(t, myFile)` |
| `MockCmd` | Records `exec.Command` calls and returns scripted output | `mock := testutil.NewMockCmd(); mock.SetOutput("modprobe", "", nil)` |
| `MockPackageManager` | In-memory `pkg.PackageManager` for package extension tests | `mgr := testutil.NewMockPackageManager("mock")` |

Extensions that do file I/O should accept an `FS extensions.FS` field in their Opts struct. When nil, `extensions.RealFS(nil)` falls back to the real OS filesystem. In tests, inject a `MapFS` to verify behavior without touching disk.

## PR Checklist

- [ ] `go test ./... -race` passes
- [ ] `go vet ./...` passes
- [ ] New code has table-driven tests
- [ ] No new dependencies without discussion
- [ ] Extension changes don't touch `internal/`
- [ ] Platform-specific files use `_linux`, `_darwin`, or `_windows` suffixes

## Release Process

Tag and push -- the [release workflow](.github/workflows/release.yml) builds MSI, deb, and pkg installers:

```bash
git tag v0.0.3
git push origin v0.0.3
```
