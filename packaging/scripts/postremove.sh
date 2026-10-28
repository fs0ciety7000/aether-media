#!/bin/sh
# Paquets .deb / .rpm : les données (/var/lib/aether*) et le compte « aether »
# sont conservés ; supprimez-les à la main si vous n'en avez plus besoin.
set -e
if [ -d /run/systemd/system ]; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi
exit 0
