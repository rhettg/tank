# 00-user-ssh (Example)

This example layer shows how to implement per-instance SSH access via a host-side
`preboot` hook. The hook edits the cloud-init user-data file to inject the
current host username and SSH public key.

## Files

- `preboot`: edits `GI_CLOUD_INIT` in place to add a user and SSH key

## Behavior

- Runs on the host before instance creation.
- Uses the host user's SSH key (`~/.ssh/id_ed25519.pub` or `~/.ssh/id_rsa.pub`).
- Adds a user with passwordless sudo and `/bin/bash` shell.

## Notes

- The hook uses a small Python script run via `uv` to merge the user into
  existing cloud-init YAML without clobbering other users.
