---
name: tank
description: >
  Usage of Tank, a CLI tool for building deterministic VM images and running disposable
  virtual machines using libvirt/KVM. Use when creating Tank projects, writing layers
  (install, firstboot, preboot hooks, files/ overlays), configuring cloud-init,
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
| `tank build [-p PATH] [--dry-run]` | Build VM image from layers |
| `tank start [name] [--cpus N] [--memory MB] [--disk SIZE] [-p PATH]` | Build + start VM |
| `tank stop [name]` | Graceful shutdown |
| `tank destroy [name]` | Stop + remove VM and instance files |
| `tank ssh [name] [-- args...]` | SSH into VM (auto-starts if needed) |
| `tank list` / `ls` / `ps` | List all instances with status/IP |

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
    │   └── files/          # Filesystem overlay copied to /
    │       └── etc/motd
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
└── instances/<name>/
    ├── disk.qcow2              # COW overlay (mutable, per-instance)
    └── cloud-init.iso
```

Qcow2 backing chain: base → build → instance overlay. Same project config always produces same build hash; cached builds skip rebuilding.

## Prerequisites

Packages: `libvirt`, `qemu-full`, `guestfs-tools`, `genisoimage` (or `mkisofs`/`xorriso`).

```bash
sudo usermod -aG libvirt $USER
sudo mkdir -p /var/lib/tank && sudo chown root:libvirt /var/lib/tank && sudo chmod 2775 /var/lib/tank
```

Uses `qemu:///system` (not session) for working networking.
