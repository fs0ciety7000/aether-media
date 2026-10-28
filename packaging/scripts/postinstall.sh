#!/bin/sh
# Paquets .deb / .rpm : dossiers de données, accès GPU, systemd.
# Appelé avec « configure [ancienne-version] » (deb) ou « 1 | 2 » (rpm).
set -e
for d in /var/lib/aether /var/lib/aether-agent; do
	mkdir -p "$d"
	chown aether:aether "$d"
	chmod 0750 "$d"
done
# Accélération matérielle (VAAPI / QSV) : /dev/dri appartient à render / video.
for g in render video; do
	if getent group "$g" >/dev/null 2>&1; then
		usermod -a -G "$g" aether || true
	fi
done
upgrade=0
case "$1" in
	configure) [ -n "$2" ] && upgrade=1 ;;
	2) upgrade=1 ;;
esac
if [ -d /run/systemd/system ]; then
	systemctl daemon-reload || true
	if [ "$upgrade" = 1 ]; then
		systemctl try-restart aether.service aether-agent.service || true
	else
		cat <<'MSG'

Aether est installé. Pour démarrer :
  serveur : sudo systemctl enable --now aether        → http://<cette-machine>:8096
  agent   : sudo systemctl enable --now aether-agent  (machine qui partage ses médias)
Réglages : /etc/aether/aether.env, /etc/aether/agent.env, /var/lib/aether/aether.toml
MSG
	fi
fi
exit 0
