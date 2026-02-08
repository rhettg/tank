#!/bin/bash
# Post-remove script for tank package.
# Cleans up UFW rules added during post-install.

set -e

if command -v ufw >/dev/null 2>&1; then
    ufw delete allow in on virbr0 to any app Tank >/dev/null 2>&1 || true

    uplink=$(ip -4 route show default 2>/dev/null | awk '{print $5; exit}')
    if [ -n "$uplink" ]; then
        ufw delete route allow in on virbr0 out on "$uplink" >/dev/null 2>&1 || true
    fi
fi
