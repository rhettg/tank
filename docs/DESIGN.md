# Tank Design Document

## Overview

Tank is a Unix-style tool for building deterministic VM images and running disposable machines using libvirt and KVM. It treats VMs with the same simplicity and disposability as containers while maintaining the benefits of real machines.

## Core Principles

1. **Filesystem-Driven**: Configuration is expressed entirely through the filesystem structure, not configuration files
2. **Deterministic**: Same inputs always produce identical images
3. **Layered Composition**: Images are built from ordered, composable layers
4. **Immutable Caching**: Base images and layer artifacts are cached and immutable
5. **Minimal Overhead**: Uses qcow2 backing chains to minimize storage and rebuild time
6. **Disposability**: VMs are cheap to create and destroy

## Architecture

### Components

#### 1. CLI (`tank` command)

The entry point for all user interactions. Operates on the current project (CWD).

**Commands:**
- `tank start [instance-suffix]` — Build image (if needed) and start VM (default: directory name)
- `tank stop [instance-suffix]` — Stop the VM
- `tank destroy [instance-suffix]` — Remove the VM completely
- `tank ssh [instance-suffix]` — SSH into the VM

**Options:**
- `--cpus N` — CPU count
- `--memory MB` — RAM in megabytes
- `--disk SIZE` — Disk size

**Examples:**
```bash
# Initialize new project
tank init --base ubuntu-24.04

# In ~/projects/web-app/
tank start                        # Instance: web-app
tank start secondary --cpus 4     # Instance: web-app-secondary
tank stop                         # Stops web-app
tank ssh secondary                # SSH into web-app-secondary
```

**Full command list:**
- `tank init [--base DISTRO]` — Scaffold new tank project
- `tank start [instance-suffix]` — Build image and start VM
- `tank stop [instance-suffix]` — Stop VM
- `tank destroy [instance-suffix]` — Remove VM
- `tank ssh [instance-suffix]` — SSH into VM

#### 2. Project Scanner

Reads the project directory structure to understand:
- Base image location/URL
- Layer definitions and execution order
- Cloud-init configuration (optional)

**Directory Structure:**
```
tank/
├── BASE                    # Base image file or URL
├── layers/
│   ├── 10-common/
│   │   ├── install         # Optional build script
│   │   └── files/          # Optional filesystem overlay
│   ├── 20-devtools/
│   └── 90-project/
└── cloud-init.yaml         # Optional (first-boot only)
```

#### 3. Image Builder

Orchestrates the image build process:

1. **Base Image Acquisition**
   - Read `BASE` file (local path or remote URL)
   - Download if needed
   - Store in `/var/lib/tank/bases/<digest>/`
   - Skip if cached

2. **Layer Application** (in lexicographic order)
   - For each layer:
     - Extract `install` if present
     - Extract `files/` directory if present
     - Create backing file for this layer's qcow2
     - Mount image, run script, apply files
     - Unmount and finalize

3. **Image Caching**
   - Cache intermediate layer artifacts in `/var/lib/tank/cache/`
   - Use content-addressed naming (hash-based)
   - Rebuild only layers that changed

4. **Final Image**
   - Store in `/var/lib/tank/images/<project-name>/`
   - Link as `current` for easy reference
   - Symlink back to specific version

#### 4. Instance Manager

Creates and manages running VMs:

1. **Instance Creation**
   - Clone final image to `/var/lib/tank/instances/<name>/disk.qcow2`
   - Create libvirt domain XML with specified resources
   - Inject cloud-init data if provided

2. **Instance Lifecycle**
   - Start: Boot VM, wait for SSH readiness
   - Stop: Graceful shutdown via libvirt
   - Destroy: Remove VM and its disk

3. **Cloud-Init Integration**
   - Auto-generate cloud-init metadata per instance
   - Inject SSH public key from `~/.ssh/id_rsa.pub`
   - Set hostname to instance name
   - Merge with user-provided `cloud-init.yaml` if present
   - Mount as CDROM device at first boot
   - Does not affect image itself

#### 5. Storage Management

**Directory Layout:**
```
/var/lib/tank/
├── bases/                          # Immutable base images
│   └── <base-digest>/
│       └── base.qcow2
├── cache/                          # Layer cache artifacts
│   └── <base-digest>/
│       └── layerchain-<hash>.qcow2
├── images/                         # Project images
│   └── <project-name>/
│       ├── <image-hash>.qcow2
│       └── current -> <image-hash>.qcow2
├── instances/                      # Running instance disks
│   └── <instance-name>/
│       ├── disk.qcow2
│       └── metadata.iso            # cloud-init metadata
└── locks/                          # Locking for concurrency
```

**Backing Chain Example:**
```
base.qcow2 (immutable)
  ↓
layer-10-common.qcow2 (cached, immutable)
  ↓
layer-20-devtools.qcow2 (cached, immutable)
  ↓
final-image.qcow2 (project image, immutable)
  ↓
instance/disk.qcow2 (mutable, per-VM)
```

### Data Flow

```
1. User runs: tank start myvm --cpus 4 --memory 8192

2. Project Scanner
   ├─ Parse project directory
   ├─ Identify base image
   └─ Enumerate layers

3. Image Builder
   ├─ Ensure base cached
   ├─ For each layer:
   │  ├─ Check cache
   │  ├─ Mount/execute if needed
   │  └─ Create qcow2 artifact
   └─ Finalize image

4. Instance Manager
   ├─ Clone image disk
   ├─ Generate cloud-init ISO
   ├─ Create libvirt domain
   └─ Boot VM

5. SSH ready
   └─ User can connect
```

## Implementation Details

### Build Process

1. **Base Image**
   - Check `BASE` file
   - If URL: download and cache
   - If local: symlink or copy
   - Store with content digest for deduplication

2. **Layer Execution**
   - Mount qcow2 as writable backing chain
   - Execute `install` if present
   - Script runs as root, full access to filesystem
   - Mount `files/` directory overlay
   - Unmount and commit

3. **Caching Strategy**
   - Hash each layer definition (script + files)
   - Cache by `(base_digest, layer_hash)`
   - Skip rebuild if hash matches
   - Detect changes: `git`, file mtimes, or explicit refresh

### Image Naming

```
Image hash: sha256(base_digest + all_layer_hashes)
Image path: /var/lib/tank/images/<project>/
Link:       current -> <image_hash>.qcow2
```

### Instance Management

1. **libvirt Integration**
   - Generate domain XML programmatically
   - Specify CPU, memory, disk, network
   - Use `qemu:///system` connection (required for NAT networking)
   - User must be in `libvirt` group for unprivileged access
   - Use NAT networking by default

2. **SSH Access**
   - Wait for port 22 to be open
   - Use cloud-init to set SSH keys
   - Connect via local IP or hostname

3. **Lifecycle**
   - Start: `virsh create`, poll SSH
   - Stop: `virsh shutdown`, timeout → `virsh destroy`
   - Destroy: `virsh undefine`, remove disk

### Locking and Concurrency

- Use file locks in `/var/lib/tank/locks/`
- Prevent concurrent builds of same image
- Allow concurrent instance creation from built images
- Lock per project name

## `tank init` Command

Scaffolds a new tank project in the current directory.

**Usage:**
```bash
tank init --base ubuntu-24.04
tank init --base fedora-41
```

**Creates:**
```
./BASE                       # URL or path to base image
./layers/
├── 10-common/
│   └── install              # SSH server, default user, basic tools
└── 90-project/
    └── install              # Empty placeholder for user customization
./cloud-init.yaml            # Optional user customization (generated template)
```

**Default `10-common/install`:**
1. Install OpenSSH server
2. Enable SSH at boot
3. Create default user (ubuntu, debian, etc. depending on base)
4. Grant sudo access
5. Set up DHCP for network

**Generated `cloud-init.yaml`:**
```yaml
# Edit this file to customize first-boot behavior
# The tool will auto-merge your config with:
# - SSH key injection
# - Hostname setting
```

## Interface

### Filesystem-Driven Configuration

**Project root:**
```
tank/              (arbitrary name)
├── BASE                # "https://..." or "./path/to/base.qcow2"
├── layers/
│   ├── 10-common/
│   │   ├── install
│   │   └── files/
│   │       └── etc/motd
│   └── 20-project/
│       └── install
└── cloud-init.yaml     (optional)
```

**install:**
1. Executed as root
2. Has full filesystem access
3. Script failure aborts build
4. Should be idempotent

**files/ overlay:**
1. Copied directly to image root (/)
2. Later layers override earlier files
3. Preserves permissions/ownership

**cloud-init.yaml (optional override):**
1. User-data for first boot only
2. Does not modify base image
3. Per-instance customization
4. Standard cloud-init format
5. Tool auto-generates base (SSH keys, hostname) and merges user config

### Commands

```bash
# Start (build if needed)
# Default instance name = $(basename $PWD)
tank start

# Additional instances from same project
tank start secondary --cpus 8 --memory 16384 --disk 100G

# Stop
tank stop
tank stop secondary

# Destroy
tank destroy
tank destroy secondary

# SSH
tank ssh
tank ssh secondary
```

**Naming Scheme:**
- Default instance: `<project-dir-name>` (e.g., `web-app`)
- Additional instances: `<project-dir-name>-<suffix>` (e.g., `web-app-secondary`)
- libvirt domains: prefixed with `tank-` for isolation (e.g., `tank-web-app`)

## Storage Efficiency

### Backing Chains

- Base image: 3-4 GB
- Per layer cache: ~100 MB - 1 GB (delta only)
- Final image: ~100 MB (delta only)
- Per instance: 40 GB (default), allocated on write

**Example**: 10 instances share same image → 10 × 40 GB allocated space, but only ~1 GB truly used for deltas.

### Content Addressing

- Bases cached by digest
- Layers cached by content hash
- Deduplicates common bases across projects
- Clean removal of unused cache

## Error Handling

- Base download fails → clear partial cache, retry
- Layer script fails → abort build, report error
- Instance creation fails → cleanup partial instance
- SSH timeout → suggest manual debugging via `virsh`

## Future Enhancements

1. **Snapshot management**: Create checkpoints of instances
2. **Multi-base support**: Multiple base images per project
3. **Network templates**: Custom networking per instance
4. **Build hooks**: Pre/post-layer callbacks
5. **Image registry**: Push/pull images from remote storage
6. **Rollback**: Revert to previous image versions

## Testing Strategy

- Unit tests for parsing and hashing
- Integration tests with libvirt (mock or local)
- End-to-end tests building real images
- Compatibility tests across distros

## Libvirt Connection: System vs Session Mode

Tank uses `qemu:///system` (system mode) rather than `qemu:///session` (session mode).

### Why not session mode?

Session mode (`qemu:///session`) runs QEMU as the current user and provides user-scoped VM management. In theory this avoids needing group membership or elevated permissions. In practice, session mode has fundamental networking limitations:

- **Bridge networking doesn't work reliably.** `qemu-bridge-helper` fails to attach tap devices to bridges, even with correct ACL configuration.
- **Firewall conflicts.** libvirt's session-mode networking generates iptables rules, but modern distributions (e.g., Arch Linux) use nftables. The rules never take effect, so NAT and forwarding don't work.
- **No DHCP/DNS connectivity.** Without working bridge or NAT networking, VMs can't get IP addresses or resolve DNS. User-mode networking (`-netdev user`) works but is slow and doesn't allow inbound connections.

These issues make session mode impractical for a tool that needs VMs to be reachable over SSH.

### System mode requirements

System mode (`qemu:///system`) runs QEMU as the `libvirt-qemu` user and lets libvirt manage networking (bridges, DHCP, DNS, firewall rules) centrally. This requires:

1. **User must be in the `libvirt` group** — for unprivileged access to the system libvirt daemon.
2. **Storage in `/var/lib/tank/`** — so the `libvirt-qemu` user can read disk images. The directory is owned by `root:libvirt` with mode `2775` (setgid), so any `libvirt` group member can write, and `libvirt-qemu` reads via world-readable file permissions.
3. **`--network default`** — uses libvirt's managed default network (virbr0 bridge with NAT, DHCP, and DNS), which works out of the box with system mode.

## Dependencies

- **libvirt**: VM management
- **qemu**: Disk image manipulation
- **cloud-init**: First-boot provisioning (optional)
- **SSH**: Remote access to VMs
