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

# Ensure libvirt default network is active and autostarted.
if command -v virsh >/dev/null 2>&1; then
    virsh -c qemu:///system net-autostart default >/dev/null 2>&1 || true
    virsh -c qemu:///system net-start default >/dev/null 2>&1 || true
fi

# Configure UFW for libvirt's default network if UFW is active.
if command -v ufw >/dev/null 2>&1; then
    if ufw status 2>/dev/null | grep -q "Status: active"; then
        if [ -f /etc/ufw/applications.d/tank ]; then
            ufw app update Tank >/dev/null 2>&1 || true
        fi
        ufw allow in on virbr0 to any app Tank >/dev/null 2>&1 || true

        uplink=$(ip -4 route show default 2>/dev/null | awk '{print $5; exit}')
        if [ -n "$uplink" ]; then
            ufw route allow in on virbr0 out on "$uplink" >/dev/null 2>&1 || true
        fi
    fi
fi
