# Graystone Development

## Build & Run
```bash
go build -o gi ./cmd/gi
./gi version
```

## Project Structure
- `cmd/gi/` - CLI entry point (cobra-based)
- `project/` - Project loading and layer detection
- `testdata/example-project/` - Example project for testing
- Version is auto-embedded from git via `runtime/debug`

## Testing
```bash
go test ./...
```

## Conventions
- Keep CLI commands in `cmd/gi/main.go` until complexity warrants splitting
- Unix philosophy: small, focused commands
- No external config files - filesystem is the interface

## Graystone Project Layout
A graystone project directory contains:
- `BASE` - URL to base cloud image
- `layers/` - Numbered directories (e.g., `10-common/`, `20-devtools/`)
  - `install.sh` - Optional provisioning script
  - `files/` - Optional file overlay (copied to VM root)
- `cloud-init.yaml` - Optional cloud-init configuration
