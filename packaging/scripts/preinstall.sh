#!/bin/sh
# Paquets .deb / .rpm : compte système « aether » (serveur et agent).
set -e
if ! getent group aether >/dev/null 2>&1; then
	groupadd --system aether
fi
if ! getent passwd aether >/dev/null 2>&1; then
	useradd --system --gid aether --home-dir /var/lib/aether --no-create-home \
		--shell /usr/sbin/nologin --comment "Aether" aether
fi
exit 0
