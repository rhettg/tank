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
See README.md for project layout, prerequisites, and storage model.
