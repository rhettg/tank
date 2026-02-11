# Volumes

Volumes give layers a way to declare persistent storage. A postgres layer says
"I need 20G at `/var/lib/postgresql`" and Tank handles creation, formatting,
attachment, and — most importantly — reattachment after a rebuild.

## Volume types

There are three kinds of volume declarations:

| Kind | Identified by | Example use |
|---|---|---|
| Block volume | has `size:` | Database data, application state |
| Root disk override | file named `root` | Larger OS disk |
| Network mount | has `source:` | NFS/9p shared storage |

All three use the same `volumes/` directory inside a layer.

## Block volumes

### Declaring a volume

Add a file to `volumes/` inside any layer:

```
layers/50-postgres/
├── install
├── firstboot
└── volumes/
    └── pgdata
```

**`pgdata`:**
```
mount: /var/lib/postgresql
size: 20G
```

That file is the entire declaration. Tank will:

1. Create a qcow2 image if one doesn't already exist
2. Attach it to the VM at boot
3. Format it (ext4 by default) if Tank just created it — **never reformats existing volumes**
4. Mount it by filesystem label at the declared path before `firstboot` scripts run

### Optional fields

```
mount: /var/lib/postgresql
size: 20G
format: ext4                # default; also supports xfs
owner: postgres:postgres    # chown the mount point after mounting
```

The `owner` field works because `install` scripts run during image build —
`apt install postgresql` creates the postgres system user in the image before
any volume is mounted.

### Where volumes are stored

```
/var/lib/tank/volumes/
└── <instance>-<name>.qcow2
```

Volumes are always named with their instance prefix. Running `tank start` and
`tank start secondary` from the same project creates independent volumes
(`myproject-pgdata.qcow2` and `secondary-pgdata.qcow2`).

### Stable device identification

Volumes are formatted with a filesystem label (`tank-<name>`) and mounted by
label, not by device path. This means adding or reordering layers won't break
existing mounts.

### Lifecycle

| Action | What happens to volumes |
|---|---|
| `tank stop` | VM stops, volumes detach |
| `tank start` | Volumes reattach automatically |
| `tank destroy` | Instance removed, **volumes preserved** |
| `tank volume rm` | Volume explicitly deleted |

The entire point: `tank destroy` + `tank start` gives you a fresh VM with your
existing data.

Adding a new volume declaration to a layer takes effect on the next
`tank destroy` + `tank start` cycle — the same workflow as any other layer change.

## Root disk sizing

The default root disk is 40G. Any layer can declare a larger root requirement:

```
layers/50-big-models/
└── volumes/
    └── root
```

**`root`:**
```
size: 200G
```

When multiple layers declare root sizes, Tank uses the **largest** value. A
machine-learning layer that needs 200G of space just says so — any project
that includes it gets the right disk size automatically.

## Network mounts

Network filesystems are declared the same way, in the same directory:

```
layers/90-nfs/
└── volumes/
    └── shared
```

**`shared`:**
```
mount: /mnt/shared
source: 192.168.1.10:/export/data
type: nfs
options: rw,soft
```

### Supported types

| Type | Description |
|---|---|
| `nfs` | NFS mount (guest-side, requires network) |
| `9p` | virtio 9p host-to-guest filesystem passthrough |
| `virtiofs` | virtiofs host-to-guest filesystem (faster than 9p) |

### How to tell them apart

The rule is simple:

- **`source:` present** → network mount (no volume created)
- **`size:` present** → block volume (qcow2 created and managed)
- **file named `root`** → root disk size override

## Composability

Volumes follow the same composition rules as layers: they are collected from all
layers in lexicographic order.

- Two layers can each declare their own volumes — you get both.
- If two layers declare a volume with the **same name**, Tank reports an error.
- Root size declarations are merged by taking the maximum.

### Reusable layers

Because volume declarations live inside layers, a shared layer carries its
storage requirements with it:

```bash
# Symlink a shared postgres layer into your project
ln -s /path/to/shared-layers/postgres layers/50-postgres

# This project now gets a persistent pgdata volume automatically
tank start
```

No extra configuration. The layer knows what it needs.

## Volume management

### `tank volume ls`

By default, lists volumes for instances associated with the current project:

```
$ tank volume ls
NAME                  SIZE   MOUNT                  INSTANCE     USED
myproject-pgdata      20G    /var/lib/postgresql    myproject    4.2G
myproject-models      100G   /models               myproject    67G
```

### `tank volume ls --all`

Lists all volumes, including those whose instances have been destroyed
(orphaned volumes):

```
$ tank volume ls --all
NAME                  SIZE   MOUNT                  INSTANCE     USED
myproject-pgdata      20G    /var/lib/postgresql    myproject    4.2G
myproject-models      100G   /models               myproject    67G
oldproject-data       50G    /data                  (orphaned)   12G
```

### `tank volume rm`

Deletes a volume by its full name (with confirmation):

```
$ tank volume rm myproject-pgdata
Remove volume myproject-pgdata (20G, /var/lib/postgresql)? [y/N] y
✓ Volume myproject-pgdata removed
```

## Example: full project with volumes

```
myproject/
├── BASE
├── cloud-init.yaml
└── layers/
    ├── 10-base/
    │   ├── install
    │   └── volumes/
    │       └── root                  # 80G root disk
    ├── 50-postgres/                  # reusable layer
    │   ├── install                   # apt install postgresql
    │   ├── firstboot                 # initdb, start service
    │   └── volumes/
    │       └── pgdata                # persistent database storage
    ├── 70-nfs/
    │   └── volumes/
    │       └── shared                # network mount
    └── 90-app/
        └── install
```

The `50-postgres` layer's `firstboot` script can assume `/var/lib/postgresql`
is already mounted and formatted — it just runs `initdb` and starts the service.
