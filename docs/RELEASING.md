# Release Process

This document describes how to release a new version of Tank.

## Prerequisites

- Push access to the repository
- All tests passing on `main` branch

## Cutting a Release

1. **Ensure tests pass** - Check that the latest commit on `main` has passing CI.

2. **Choose a version** - Follow [semantic versioning](https://semver.org/):
   - `MAJOR.MINOR.PATCH` (e.g., `v1.2.3`)
   - Bump MAJOR for breaking changes
   - Bump MINOR for new features
   - Bump PATCH for bug fixes

3. **Create and push the tag**:
   ```bash
   git checkout main
   git pull origin main
   git tag v1.0.0
   git push origin v1.0.0
   ```

4. **Monitor the release** - The [Release workflow](../.github/workflows/release.yaml) will automatically:
   - Build binaries for Linux and macOS (amd64 and arm64)
   - Create `.deb` and `.rpm` packages
   - Create a GitHub Release with all artifacts

5. **Verify the release** - Check the [Releases page](https://github.com/rhettg/tank/releases) for the new version.

## What Gets Built

GoReleaser produces the following artifacts:

| Platform | Architectures | Formats |
|----------|---------------|---------|
| Linux    | amd64, arm64  | tar.gz, .deb, .rpm |
| macOS    | amd64, arm64  | tar.gz |

## Installing from a Release

### Direct download

```bash
# Download and extract
curl -LO https://github.com/rhettg/tank/releases/download/v1.0.0/tank_1.0.0_linux_amd64.tar.gz
tar xzf tank_1.0.0_linux_amd64.tar.gz
sudo mv tank /usr/local/bin/
```

### Debian/Ubuntu

```bash
curl -LO https://github.com/rhettg/tank/releases/download/v1.0.0/tank_1.0.0_amd64.deb
sudo dpkg -i tank_1.0.0_amd64.deb
```

### RHEL/Fedora

```bash
curl -LO https://github.com/rhettg/tank/releases/download/v1.0.0/tank_1.0.0_amd64.rpm
sudo rpm -i tank_1.0.0_amd64.rpm
```

## Local Testing

To test the release process locally (without publishing):

```bash
goreleaser release --snapshot --clean
```

This creates artifacts in the `dist/` directory.
