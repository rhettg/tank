# Graystone Development

## Build & Run
```bash
go build -o gi ./cmd/gi
./gi version
```

## Project Structure
- `cmd/gi/` - CLI entry point (cobra-based)
- Version is auto-embedded from git via `runtime/debug`

## Conventions
- Keep CLI commands in `cmd/gi/main.go` until complexity warrants splitting
- Unix philosophy: small, focused commands
- No external config files - filesystem is the interface
