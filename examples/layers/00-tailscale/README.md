# tailscale

Tailscale layer that automatically connects VMs to your tailnet using OAuth-generated ephemeral auth keys.

## How it works

1. **preboot** (host-side): Uses Tailscale OAuth API to generate a short-lived ephemeral auth key and injects it into cloud-init
2. **install.sh** (image build): Installs Tailscale package
3. **firstboot.sh** (VM boot): Authenticates to Tailscale using the injected auth key

## Prerequisites

### Create an OAuth Client

1. Go to [Tailscale Admin Console → Settings → OAuth clients](https://login.tailscale.com/admin/settings/oauth)
2. Create a new OAuth client with **auth_keys** scope
3. Select the tags you want to assign to VMs (e.g., `tag:server`)
4. Save the Client ID and Client Secret

### Define Tags in ACL

Your tailnet's ACL policy must define the tags. Example:

```json
{
  "tagOwners": {
    "tag:server": ["autogroup:admin"]
  }
}
```

## Environment Variables

Set these before running `gi start`:

| Variable | Required | Description |
|----------|----------|-------------|
| `TAILSCALE_CLIENT_ID` | Yes | OAuth client ID |
| `TAILSCALE_CLIENT_SECRET` | Yes | OAuth client secret |
| `TAILSCALE_TAGS` | Yes | Comma-separated tags (e.g., `tag:server,tag:vm`) |
| `TAILSCALE_API_URL` | No | API base URL (default: `https://api.tailscale.com`) |

## Usage

Symlink into your project's layers directory:

```bash
ln -s /path/to/examples/layers/00-tailscale layers/50-tailscale
```

Set environment variables and start:

```bash
export TAILSCALE_CLIENT_ID="your-client-id"
export TAILSCALE_CLIENT_SECRET="your-client-secret"
export TAILSCALE_TAGS="tag:server"

tank start
```

The VM will automatically appear in your tailnet as an ephemeral node.

## Security Notes

- Auth keys are ephemeral and expire after 1 hour
- The key is stored only in RAM (`/run/tank/tailscale.env`) and not persisted to disk
- VMs are registered as ephemeral nodes and auto-removed when offline
- OAuth client secrets should be stored securely (e.g., password manager, secrets vault)
