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

* **`tank init <base-url>`** — Initialize a new project with BASE, cloud-init, and a starter layer
* **`tank start [name]`** — Build image (if needed) and start the VM
* **`tank stop [name]`** — Stop the VM
* **`tank destroy [name]`** — Stop and remove the VM completely
* **`tank ssh [name]`** — Connect to the VM over SSH
* **`tank status [name]`** — Show project status: instance state, IP, build cache, image freshness, layers, and volumes
* **`tank list`** — List all instances with status and IP
* **`tank build`** — Build the VM image without starting
* **`tank layers`** — List layers with content hashes
* **`tank volume ls [--all]`** — List persistent volumes
* **`tank volume rm <name>`** — Remove a persistent volume

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
* `install` — executed during image build (executable)
* `firstboot` — executed on first VM boot (executable)
* `files/` — filesystem overlay copied verbatim
* `volumes/` — persistent storage and network mount declarations

Example:

```
layers/
├── 10-common/
│   ├── install
│   └── files/
│       └── etc/
│           └── motd
├── 20-devtools/
│   ├── install
│   └── files/
│       └── usr/
│           └── local/
│               └── bin/
└── 90-project/
    └── install
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
* `genisoimage` (or `mkisofs`/`xorriso` for cloud-init ISOs)

### Guestfs appliance cache

Tank relies on `virt-customize` (libguestfs). On some distros (notably Ubuntu),
libguestfs cannot build its supermin appliance without a readable host kernel.
Tank will try to use a fixed appliance in this order:

* Cached appliance in `/var/lib/tank/guestfs/`
* System appliance (eg. `/usr/lib/libguestfs/appliance`)
* Build a fixed appliance with `libguestfs-make-fixed-appliance`
* Download a prebuilt appliance from `download.libguestfs.org`

You can override the appliance path by setting `LIBGUESTFS_PATH` before running
`tank`.

### Groups

Your user must be in the `libvirt` and `kvm` groups:

```bash
sudo usermod -aG libvirt,kvm $USER
```

Log out and back in for the groups to take effect.

The `libvirt` group grants access to `virsh` and the system connection. The `kvm`
group grants access to `/dev/kvm` for hardware-accelerated virtualization — without
it, QEMU falls back to software emulation (TCG), which is dramatically slower.

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

### Networking + firewall

Tank uses libvirt's default network (`virbr0`) for DHCP, DNS, and NAT. If your host
firewall blocks DHCP or routed traffic, VMs will boot but never get an IP address.

The Debian/RPM packages apply the following on install:

* ensure libvirt's `default` network is started and autostarted
* add a UFW profile allowing DHCP/DNS on `virbr0`
* add a routed UFW rule so guests can reach the outside world

If you use UFW manually, the equivalent commands are:

```bash
sudo ufw app update Tank
sudo ufw allow in on virbr0 to any app Tank
sudo ufw route allow in on virbr0 out on <uplink>
```

Replace `<uplink>` with your host's outbound interface (e.g., `eth0` or `wlan0`).

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

## Volumes: persistent storage that survives rebuilds

Layers can declare **persistent volumes** that are created, formatted, and mounted
automatically. Destroy a VM, rebuild it, start it again — your data is still there.

### Layer volumes

Add a `volumes/` directory to any layer. Each file declares one volume:

```
layers/50-postgres/
├── install
├── firstboot
└── volumes/
    └── pgdata
```

**`layers/50-postgres/volumes/pgdata`:**
```
mount: /var/lib/postgresql
size: 20G
```

That's it. Tank will:

* create the qcow2 volume if it doesn't exist
* attach it to the VM
* format and mount it before your `firstboot` script runs

The postgres layer can be **symlinked into any project** and it brings its
storage requirement with it.

### Root disk sizing

Any layer can declare a root disk size requirement:

```
layers/50-big-models/
└── volumes/
    └── root
```

**`layers/50-big-models/volumes/root`:**
```
size: 200G
```

When multiple layers declare root sizes, Tank uses the largest value. A
machine-learning layer that needs 200G just says so — any project that
includes it gets the right disk size.

### Network mounts

Network filesystems use the same `volumes/` directory:

```
layers/90-nfs/volumes/shared
  mount: /mnt/shared
  source: 192.168.1.10:/export/data
  type: nfs
  options: rw,soft
```

If the file has a `source:`, it's a network mount. If it has a `size:`, it's
a block volume.

### The rebuild experience

```
$ tank destroy
▸ Force stopping VM tank-myproject
▸ Removing instance files
✓ Instance myproject destroyed
  Persistent volumes retained: pgdata (20G)

$ tank start
▸ Creating overlay disk
▸ Reattaching volume pgdata (20G) → /var/lib/postgresql
▸ Starting VM tank-myproject
✓ Instance myproject started
```

### Volume management

```bash
tank volume ls                    # volumes for instances in this project
tank volume ls --all              # all volumes, including orphaned
tank volume rm myproject-pgdata   # delete a volume (with confirmation)
```

See [docs/VOLUMES.md](docs/VOLUMES.md) for the full volume reference.

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
