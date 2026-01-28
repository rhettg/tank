# Graystone

**Deterministic VM images. Disposable machines. Built for libvirt.**

Graystone Industries (`gi`) is an opinionated, Unix-style tool for **building and running virtual machines locally** using **libvirt and KVM**.

If you want VMs that feel as cheap and repeatable as containers—but remain real machines—Graystone is for you.

---

## What Graystone does

Graystone has two responsibilities:

1. **Build immutable VM images** from files and shell scripts
2. **Run disposable virtual machines** from those images using libvirt

---

## Command reference

* **`gi start [name]`** — Build image (if needed) and start the VM (default name: project directory)
* **`gi stop [name]`** — Stop the VM (default name: project directory)
* **`gi destroy [name]`** — Stop and remove the VM completely (default name: project directory)
* **`gi ssh [name]`** — Connect to the VM over SSH (default name: project directory)

Run multiple instances from the same image:

```bash
gi start                     # uses directory name, e.g. "myproject"
gi start secondary --cpus 4  # custom name "secondary"
gi start dev --memory 8192   # custom name "dev"
```

Optional arguments to `gi start`:

* `--cpus N` — CPU count (default: 2)
* `--memory MB` — RAM in MB (default: 4096)
* `--disk SIZE` — Disk size (default: 40G)

## The filesystem *is* the interface

Graystone projects are driven entirely by the filesystem.

A minimal project:

```
graystone/
├── BASE
├── layers/
│   ├── 10-common/
│   ├── 20-devtools/
│   └── 90-project/
└── cloud-init.yaml   # optional
```

---

## Base images (explicit and pinned)

Every image has a **BASE** layer.

The `BASE` file can be:

* a qcow2 file (or symlink)
* a text file containing a remote URL (`https://cloud-images.ubuntu.com/releases/24.04/release/ubuntu-24.04-server-cloudimg-amd64.img`)`

Bases are:

* downloaded or imported once
* cached locally
* immutable

---

## Layers: composable, ordered, filesystem-driven

Layers are directories under `layers/`.

Each layer may contain:

* `install.sh` — executed during image build
* `firstboot.sh` — executed on first VM boot
* `files/` — filesystem overlay copied verbatim

Example:

```
layers/
├── 10-common/
│   ├── install.sh
│   └── files/
│       └── etc/
│           └── motd
├── 20-devtools/
│   ├── install.sh
│   └── files/
│       └── usr/
│           └── local/
│               └── bin/
└── 90-project/
    └── install.sh
```

### Composition rules

* Layers are applied **in lexicographic order**
* Later layers override earlier files
* Scripts execute in the same order
* Everything is deterministic

There is no hidden merge logic—just filesystem semantics.

## Prerequisites

Graystone requires **libvirt**, **QEMU/KVM**, and uses `qemu:///system` for VM management.

### System packages

- `libvirt`
- `qemu-full` (or equivalent)
- `guestfs-tools` (provides `virt-customize` for applying layers)
- `genisoimage` (for cloud-init ISOs)

### Groups

Your user must be in the `libvirt` group:

```bash
sudo usermod -aG libvirt $USER
```

Log out and back in for the group to take effect.

### Storage directory

Graystone stores images and instances in `/var/lib/graystone`. Create it with:

```bash
sudo mkdir -p /var/lib/graystone
sudo chown root:libvirt /var/lib/graystone
sudo chmod 2775 /var/lib/graystone
```

This gives:
- `root` ownership (conventional for `/var/lib`)
- `libvirt` group with write access (so any `libvirt` group member can run `gi`)
- Setgid bit so new files/directories inherit the `libvirt` group
- `libvirt-qemu` (the user QEMU runs as under system mode) can read images via world-readable permissions

---

## Storage model (qcow2 backing chains)

Graystone stores everything under `/var/lib/graystone/`.

```
/var/lib/graystone/
├── images/
│   └── <base-image-name>.img
├── builds/
│   └── <project-hash>.qcow2
└── instances/
    └── <instance-name>/
        ├── disk.qcow2
        └── cloud-init.iso
```

### Instance disks

Each instance gets a copy-on-write overlay:

```
builds/<project-hash>.qcow2  (immutable, shared)
  ↑
instances/<name>/disk.qcow2  (mutable, per-instance)
```

This allows:

* multiple instances from the same build
* fast instance creation
* changes isolated per instance

---

## Optional: cloud-init

Cloud-init is supported **only for first-boot identity**:

* SSH keys
* hostnames
* per-instance users

Images remain reusable.
Instances remain unique.

---

## Philosophy

> Machines are cheap.
> Images are intentional.
> Rebuild instead of repair.

Graystone brings container-style ergonomics back to virtual machines—without pretending VMs are containers.

---
