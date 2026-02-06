#!/bin/bash
# Post-install script for tank package.
# Creates /var/lib/tank with libvirt group ownership so that
# users in the libvirt group can manage VM images without sudo.

set -e

TANK_DIR="/var/lib/tank"

if [ ! -d "$TANK_DIR" ]; then
    mkdir -p "$TANK_DIR"
fi

# Set ownership and permissions:
# - root:libvirt ownership
# - 2775 = setgid + rwxrwxr-x (new files inherit libvirt group)
if getent group libvirt >/dev/null 2>&1; then
    chown root:libvirt "$TANK_DIR"
    chmod 2775 "$TANK_DIR"
fi
