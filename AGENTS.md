# Tank Development

Tank is a CLI tool for **building deterministic VM images** and **running disposable virtual machines** using libvirt/KVM. It brings container-style ergonomics to virtual machines — filesystem-driven projects, layered image builds, and instant disposable instances.

```
Build and run virtual machines using libvirt and KVM.

Usage:
  tank [command]

Available Commands:
  build       Build a VM image from project layers
  destroy     Stop and remove the VM completely
  init        Initialize a new tank project
  layers      List project layers and their content hashes
  ls          List all VM instances
  ssh         SSH into a running VM (auto-starts if needed)
  start       Start the VM (builds image if needed)
  status      Show project status
  stop        Stop the VM
  version     Print version information
  volume      Manage persistent volumes

Flags:
  -p, --project string   path to project directory (default ".")
```

## Build & Run
```bash
go build -o tank ./cmd/tank
./tank version
```

## Testing
```bash
go test ./...
```

## Project Structure
- `cmd/tank/` - CLI entry point (cobra-based, single `main.go`)
- `build/` - Image building: virt-customize orchestration, guestfs appliance management, overlay creation, preflight checks, disk resizing
- `instance/` - Instance lifecycle: create/start/stop/destroy, preboot hooks, volume attachment
- `project/` - Project loading: layer detection, content hashing, volume declarations, env file parsing
- `ui/` - Terminal UI: progress indicators, styled output, table formatting
- `share/` - Embedded shared layers (bundled via `embed`)
- `scripts/` - Packaging scripts: postinstall/postremove hooks, UFW profiles, Arch PKGBUILD
- `skills/` - Amp skill for Tank usage
- `docs/` - Design docs, volume reference, preboot hooks, releasing
- `testdata/example-project/` - Example tank project for testing
- `.github/workflows/` - CI workflows
- `.goreleaser.yaml` - Release configuration

## Conventions
- Keep CLI commands in `cmd/tank/main.go` until complexity warrants splitting
- Unix philosophy: small, focused commands
- No external config files - filesystem is the interface
- Version is auto-embedded from git via `runtime/debug`
- See `DESIGN.md` for CLI/UI design system conventions, command naming, and output patterns

## Tank Project Layout
See README.md for project layout, prerequisites, and storage model.
