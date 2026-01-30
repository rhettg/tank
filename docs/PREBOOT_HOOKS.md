# Preboot Hooks and Cloud-Init Editing

## Summary

Introduce a host-side layer hook, `preboot`, that runs before instance creation.
The hook receives a writable cloud-init file path via `GI_CLOUD_INIT` and can edit
it in place. This enables workflows like Tailscale OAuth key generation and secret
injection without adding Tailscale-specific logic to Graystone core.

## Goals

- Provide a host-side hook for per-instance logic.
- Allow layers to edit cloud-init before ISO generation.
- Keep the model filesystem-driven and layer-ordered.
- Avoid long-lived secrets inside VM images.

## Non-Goals

- Full secret-management system.
- Remote provisioning beyond local host execution.
- Complex cloud-init merging logic in core.

## Proposed Behavior

### New Layer Hook

Layers may include `preboot` alongside `install.sh`, `firstboot.sh`, and `files/`.

Execution order follows existing layer ordering (lexicographic).
Each `preboot` runs on the host before the VM is created.

### Environment Variables

Expose the following environment variables to `preboot`:

- `GI_PROJECT_ROOT`: absolute path to project root
- `GI_INSTANCE_NAME`: resolved instance name
- `GI_LAYER_PATH`: absolute path to the current layer
- `GI_CLOUD_INIT`: writable path to the instance cloud-init user-data file
- `GI_WORK_DIR`: temporary directory for hook scratch files

### Cloud-Init Flow

1. `gi start` resolves the instance name.
2. Graystone assembles base cloud-init:
   - default user + SSH key content
   - project `cloud-init.yaml` (if present)
3. Graystone writes the merged content to a temp file and sets `GI_CLOUD_INIT`.
4. `preboot` hooks run in layer order and edit `GI_CLOUD_INIT` in place.
5. Graystone reads `GI_CLOUD_INIT` and generates the cloud-init ISO.

### Error Handling

- If a `preboot` hook exits non-zero, abort `gi start` and surface its output.

## Tailscale Example

### Layer Layout

```
layers/
└── 40-tailscale/
    ├── install.sh
    ├── preboot
    └── firstboot.sh
```

### `preboot` (host-side)

- Perform OAuth client flow against Tailscale API.
- Create ephemeral, tagged auth key.
- Inject the key into `GI_CLOUD_INIT` using `write_files`:

```
write_files:
  - path: /run/gi/secrets.env
    permissions: "0600"
    content: |
      TAILSCALE_AUTH_KEY=tskey-...
```

### `firstboot.sh` (guest-side)

- Source `/run/gi/secrets.env`.
- Run `tailscale up --auth-key "$TAILSCALE_AUTH_KEY" --hostname "$HOSTNAME"`.

## Implementation Sketch

### Project Scanner

- Extend the `Layer` struct to include `HasPreboot`.
- Detect `preboot` in each layer directory during project load.

### Instance Creation Path

- Build base cloud-init content (existing logic).
- Write to a temp file (`GI_CLOUD_INIT`).
- For each layer with `preboot`:
  - Run the script on the host with env vars listed above.
- Read the updated cloud-init file and pass it to instance creation.

### CLI Output

- Print a step for each `preboot` executed.

## Open Questions

- Should we provide a helper for YAML merging or expect hooks to manage it?
- Should `GI_WORK_DIR` be created per-instance or per-hook?
- Should a preboot hook be allowed to remove cloud-init entirely?

## Definition of Done

- `preboot` is detected and executed in layer order.
- `GI_CLOUD_INIT` is editable and reflected in the final cloud-init ISO.
- Failures in preboot hooks stop `gi start` with clear errors.
- Documentation updated to describe the new hook and use case.
