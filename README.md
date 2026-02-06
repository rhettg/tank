# Tank

**Deterministic VM images. Disposable machines. Built for libvirt.**

Tank is an opinionated, Unix-style tool for **building and running virtual machines locally** using **libvirt and KVM**.

If you want VMs that feel as cheap and repeatable as containers—but remain real machines—Tank is for you.

---

## What Tank does

Tank has two responsibilities:

1. **Build immutable VM images** from files and shell scripts
2. **Run disposable virtual machines** from those images using libvirt

---

## Command reference

* **`tank start [name]`** — Build image (if needed) and start the VM
* **`tank stop [name]`** — Stop the VM
* **`tank destroy [name]`** — Stop and remove the VM completely
* **`tank ssh [name]`** — Connect to the VM over SSH

Run multiple instances from the same image:

```bash
tank start                     # uses directory name, e.g. "myproject"
tank start secondary --cpus 4  # custom name "secondary"
tank start dev --memory 8192   # custom name "dev"
```

Optional arguments to `tank start`:

* `--cpus N` — CPU count (default: 2)
* `--memory MB` — RAM in MB (default: 4096)
* `--disk SIZE` — Disk size (default: 40G)

## The filesystem *is* the interface

Tank projects are driven entirely by the filesystem.

A minimal project:

```
myproject/
├── BASE
├── layers/
│   ├── 10-common/
│   ├── 20-devtools/
│   └── 90-project/
└── cloud-init.yaml
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

* `preboot` — executed on the host before instance creation (can edit cloud-init)
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

Tank requires **libvirt**, **QEMU/KVM**, and uses `qemu:///system` for VM management.

### System packages

* `libvirt` (provides `virsh`)
* `qemu-full` (or equivalent, provides `qemu-img`)
* `guestfs-tools` (provides `virt-customize` for applying layers)
* `virt-install` (deb: `virtinst`, rpm: `virt-install`)
* `genisoimage` (for cloud-init ISOs)

### Groups

Your user must be in the `libvirt` group:

```bash
sudo usermod -aG libvirt $USER
```

Log out and back in for the group to take effect.

### Storage directory

Tank stores images and instances in `/var/lib/tank`. Create it with:

```bash
sudo mkdir -p /var/lib/tank
sudo chown root:libvirt /var/lib/tank
sudo chmod 2775 /var/lib/tank
```

This gives:

* `root` ownership (conventional for `/var/lib`)
* `libvirt` group with write access (so any `libvirt` group member can run `tank`)
* Setgid bit so new files/directories inherit the `libvirt` group
* `libvirt-qemu` (the user QEMU runs as under system mode) can read images via world-readable permissions

---

## Storage model (qcow2 backing chains)

Tank stores everything under `/var/lib/tank/`.

```
/var/lib/tank/
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

Layers can also include a `preboot` host hook to edit the generated cloud-init
before the VM boots (for example, to inject short-lived secrets).

### Preboot hooks

The `preboot` script runs on the host before instance creation. It receives:

* `TANK_PROJECT_ROOT` — absolute path to project root
* `TANK_INSTANCE_NAME` — resolved instance name
* `TANK_LAYER_PATH` — absolute path to the current layer
* `TANK_CLOUD_INIT` — writable path to the cloud-init user-data file
* `TANK_WORK_DIR` — temporary directory for hook scratch files

Hooks run in layer order and can edit `TANK_CLOUD_INIT` in place. If a hook exits
non-zero, `tank start` aborts with an error.

Images remain reusable.
Instances remain unique.

---

## Claude Code

Tank includes a [Claude Code](https://docs.anthropic.com/en/docs/claude-code) skill. Install it with:

```bash
npx skills add https://github.com/rhettg/tank
```

---

## Philosophy

> Machines are cheap.
> Images are intentional.
> Rebuild instead of repair.

Tank brings container-style ergonomics to virtual machines—without pretending VMs are containers.

---
