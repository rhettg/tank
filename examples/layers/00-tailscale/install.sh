#!/bin/bash
# Install Tailscale during image build
set -e

curl -fsSL https://tailscale.com/install.sh | sh

# Enable the service but don't start it (no auth key yet)
systemctl enable tailscaled
