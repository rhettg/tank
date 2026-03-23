# Build Cache Garbage Collection

Tank stores immutable cached build artifacts under `/var/lib/tank/builds/`.
These are qcow2 backing-chain nodes, not standalone Docker-style layers.

## What Tank prunes

Tank prunes only **cached build artifacts** in `builds/` that are:

* unreachable from any GC root
* not pinned

Tank does **not** prune:

* persistent volumes
* base images in `images/`
* instance disks in `instances/`

## GC roots

A cached build is kept if it is reachable from any of these roots:

* the latest recorded build for a project
* a build currently backing an instance disk
* a pinned build

Reachability is then followed through the qcow2 backing chain, so parent stages
needed by a kept build are also retained.

## Automatic prune after build

After a successful `tank build` or `tank start`, Tank automatically runs the
same reachability-based prune logic as the manual `tank prune --apply` command.

This means superseded cached builds disappear automatically once they are both:

* unreachable
* unpinned

If a build is still needed by an instance or has been pinned explicitly, it is
retained.

## Manual commands

Show reclaimable cached builds without deleting them:

```bash
tank prune
```

Delete unreachable cached builds immediately:

```bash
tank prune --apply
```

Explain why a build is kept or reclaimable:

```bash
tank prune --explain <hash>
```

Keep a build even if nothing currently references it:

```bash
tank pin <hash>
```

Remove that override:

```bash
tank unpin <hash>
```

## Pins

Pins are explicit user intent. A pinned build becomes a GC root and is kept
until you remove the pin.

Pins are useful when you want to preserve:

* a known-good build for rollback
* a build tied to debugging or a bug report
* a demo or milestone snapshot

## Volumes

Persistent volumes are outside the build-cache GC system. Rebuilding images,
running `tank prune`, or automatic post-build prune does not delete user
volumes.
