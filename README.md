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

* **`gi start [name]`** — Build image (if needed) and start the VM (default name: `default`)
* **`gi stop [name]`** — Stop the VM (default name: `default`)
* **`gi destroy [name]`** — Stop and remove the VM completely (default name: `default`)
* **`gi ssh [name]`** — Connect to the VM over SSH (default name: `default`)

Run multiple instances from the same image:

```bash
gi start primary
gi start secondary --cpus 4
gi start dev --memory 8192
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

## Storage model (qcow2 backing chains)

Graystone stores everything as files on disk.

```
~/.graystone/
├── bases/
│   └── ubuntu-24.04-<digest>/base.qcow2
├── cache/
│   └── <base-digest>/
│       └── layerchain-<hash>.qcow2
├── images/
│   └── default/
│       ├── <image-hash>.qcow2
│       └── current -> <image-hash>.qcow2
├── instances/
│   └── default/
│       └── disk.qcow2
└── locks/
```

### Layer caching

Each layer can produce a **cached qcow2 artifact** using backing files:

```
base.qcow2
  ↑
layer-10-common.qcow2
  ↑
layer-20-devtools.qcow2
  ↑
final-image.qcow2
  ↑
instance disk (mutable)
```

This allows:

* fast rebuilds when only later layers change
* minimal disk usage
* native libvirt cloning

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
