#!/bin/sh
# Paquets .deb / .rpm : arrêt des services à la désinstallation (pas à la mise à jour).
# Appelé avec « remove | upgrade » (deb) ou « 0 | 1 » (rpm).
set -e
case "$1" in
	remove|0)
		if [ -d /run/systemd/system ]; then
			systemctl disable --now aether.service aether-agent.service >/dev/null 2>&1 || true
		fi
		;;
esac
exit 0
