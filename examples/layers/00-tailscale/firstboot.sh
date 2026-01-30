#!/bin/bash
# Authenticate Tailscale on first boot using the injected auth key
set -e

SECRETS_FILE="/run/gi/tailscale.env"

if [ ! -f "$SECRETS_FILE" ]; then
    echo "Error: $SECRETS_FILE not found - preboot hook may have failed"
    exit 1
fi

# Source the secrets
source "$SECRETS_FILE"

if [ -z "$TAILSCALE_AUTH_KEY" ]; then
    echo "Error: TAILSCALE_AUTH_KEY not set in $SECRETS_FILE"
    exit 1
fi

# Start tailscaled if not already running
systemctl start tailscaled

# Wait for tailscaled to be ready
sleep 2

# Authenticate with the ephemeral auth key
# The key is already tagged and ephemeral, so we just need to bring it up
tailscale up --auth-key="$TAILSCALE_AUTH_KEY"

echo "Tailscale connected!"
tailscale status
