---
name: tank
description: >
  Usage of Tank, a CLI tool for building deterministic VM images and running disposable
  virtual machines using libvirt/KVM. Use when creating Tank projects, writing layers
  (install.sh, firstboot.sh, preboot hooks, files/ overlays), configuring cloud-init,
  managing VM instances (start, stop, destroy, ssh), or troubleshooting libvirt/qemu issues.
  Triggers: tank commands, VM image building, layer creation, cloud-init configuration,
  preboot hooks, BASE file setup, .env files, libvirt/qemu/virt-customize issues.
---

# Tank

Deterministic VM images. Disposable machines. Built for libvirt/KVM.

## Quick Start

```bash
tank init https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
# edit layers as needed
tank start
tank ssh
```

## Commands

| Command | Description |
|---------|-------------|
| `tank init <base-url>` | Initialize project with BASE, cloud-init.yaml, starter layer |
| `tank layers [-p PATH]` | List layers with content hashes |
| `tank build [-p PATH] [--no-cache] [--dry-run]` | Build VM image from layers |
| `tank start [name] [--cpus N] [--memory MB] [--disk SIZE] [--no-cache] [-p PATH]` | Build + start VM |
| `tank stop [name]` | Graceful shutdown |
| `tank destroy [name]` | Stop + remove VM and instance files |
| `tank ssh [name] [-- args...]` | SSH into VM (auto-starts if needed) |
| `tank status [name]` | Show project status: instance state, IP, build cache, layers, volumes |
| `tank ls` / `list` / `ps` | List all instances with status/IP |
| `tank volume ls [--all]` | List volumes (project or all) |
| `tank volume rm <name>` | Remove a persistent volume |

Instance name defaults to project directory basename. Multiple instances from same image:

```bash
tank start                     # "myproject"
tank start secondary --cpus 4  # "secondary"
```

## Project Layout

```
myproject/
├── BASE                    # Required: base image URL or local path
├── cloud-init.yaml         # Optional: cloud-init customization
├── .env                    # Optional: env vars for preboot hooks
└── layers/
    ├── 10-common/
    │   ├── install         # Runs as root during build (virt-customize)
    │   ├── firstboot       # Runs inside VM on first boot
    │   ├── preboot         # Runs on HOST before VM creation
    │   ├── files/          # Filesystem overlay copied to /
    │   │   └── etc/motd
    │   └── volumes/        # Persistent storage declarations
    │       └── pgdata
    ├── 20-devtools/
    │   └── install
    └── 90-project/
        └── install
```

- No config files — the filesystem is the interface.
- Layers apply in **lexicographic order**.
- `files/` contents copy directly to image root `/`. Later layers override earlier files.
- `install` runs as root inside the image during build. Keep idempotent.
- `firstboot` runs inside the VM on first boot (via cloud-init). Does not affect the image.
- `preboot` runs on the **host** before VM creation.

## Volumes

Layers can declare **persistent volumes** that survive VM destroys and rebuilds.

### Block volumes

Add a file to `volumes/` in any layer:

```
# layers/50-postgres/volumes/pgdata
mount: /var/lib/postgresql
size: 20G
```

Tank will create the qcow2 volume if it doesn't exist, attach it to the VM, and format/mount it before `firstboot` runs.

### Root disk sizing

Any layer can declare a root disk size:

```
# layers/50-big-models/volumes/root
size: 200G
```

When multiple layers declare root sizes, Tank uses the largest. Default is 50G (override with `TANK_BUILD_ROOT_SIZE`).

### Network mounts

```
# layers/90-nfs/volumes/shared
mount: /mnt/shared
source: 192.168.1.10:/export/data
type: nfs
options: rw,soft
```

If a volume file has `source:`, it's a network mount. If it has `size:`, it's a block volume.

### Volume management

```bash
tank volume ls                    # volumes for this project's instances
tank volume ls --all              # all volumes, including orphaned
tank volume rm myproject-pgdata   # delete a volume (with confirmation)
```

## Preboot Hooks

Executable `preboot` in a layer directory. Useful for injecting secrets into cloud-init.

**Environment variables:**
- `TANK_PROJECT_ROOT` — absolute path to project root
- `TANK_INSTANCE_NAME` — resolved instance name
- `TANK_LAYER_PATH` — absolute path to current layer
- `TANK_CLOUD_INIT` — writable cloud-init file (edit in place)
- `TANK_WORK_DIR` — temporary scratch directory
- Plus variables from `.env` file

Non-zero exit aborts `tank start`.

## Storage

```
/var/lib/tank/
├── images/<base>.img           # Cached base images (immutable)
├── builds/<hash>.qcow2         # Build images (immutable, shared)
├── volumes/<name>.qcow2        # Persistent volumes
└── instances/<name>/
    ├── disk.qcow2              # COW overlay (mutable, per-instance)
    └── cloud-init.iso
```

Qcow2 backing chain: base → build → instance overlay. Same project config always produces same build hash; cached builds skip rebuilding.

## Prerequisites

Packages: `libvirt`, `qemu-full`, `guestfs-tools`, `virt-install`, `genisoimage` (or `mkisofs`/`xorriso`).

```bash
sudo usermod -aG libvirt,kvm $USER
sudo mkdir -p /var/lib/tank && sudo chown root:libvirt /var/lib/tank && sudo chmod 2775 /var/lib/tank
```

Uses `qemu:///system` (not session) for working networking.
