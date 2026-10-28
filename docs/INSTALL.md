# Installer Aether

Aether tient dans un seul binaire, `aether`, qui fait office de **serveur** (`aether serve`, interface sur le port 8096) et d'**agent** (`aether agent`, port 8097) : l'agent partage les médias d'une autre machine avec le serveur, sans base de données ni configuration.

| Méthode | Pour qui | Section |
|---|---|---|
| Docker / Compose | NAS, serveur Linux, accélération GPU | [Docker](#docker--compose) |
| Paquet `.deb` / `.rpm` | Debian, Ubuntu, Fedora, RHEL… | [Paquets Linux](#paquets-deb-et-rpm) |
| Homebrew | macOS (et Linux) | [Homebrew](#homebrew) |
| Archive `.zip` | Windows 10 / 11, Windows Server | [Windows](#windows) |
| Binaire seul | toute autre machine | [Binaire manuel](#binaire-manuel) |

Les binaires, paquets, sommes de contrôle (`checksums.txt`) et SBOM sont publiés sur la page [Releases](https://github.com/fs0ciety7000/aether-media/releases). Plateformes : Linux amd64 / arm64 / armv7, macOS Intel / Apple Silicon, Windows amd64. Dans les exemples, remplacez `0.1.0` par la version voulue.

**FFmpeg** est nécessaire pour analyser les fichiers et transcoder. L'image Docker l'inclut (jellyfin-ffmpeg) ; ailleurs, installez-le avec le gestionnaire de paquets du système. Le build de Jellyfin (`jellyfin-ffmpeg7`) est recommandé : il contient `zscale` (conversion HDR → SDR fidèle), libass (sous-titres incrustés) et VAAPI / QSV / NVENC.

Après l'installation, ouvrez `http://<machine>:8096` : l'assistant de premier lancement crée le compte administrateur.

---

## Docker / Compose

Image multi-arch (`linux/amd64`, `linux/arm64`) : `ghcr.io/fs0ciety7000/aether`. Elle contient jellyfin-ffmpeg 7 (VAAPI, QSV, NVENC, zscale/tonemap, libass), tourne sous l'utilisateur non root `1000`, garde ses données dans `/data` et expose un healthcheck (`aether healthcheck`).

```yaml
# compose.yml
services:
  aether:
    image: ghcr.io/fs0ciety7000/aether:latest
    container_name: aether
    user: "1000:1000"              # propriétaire de ./data
    ports:
      - "8096:8096"
    environment:
      TZ: Europe/Paris
    volumes:
      - ./data:/data
      - /srv/media/films:/media/films:ro
      - /srv/media/series:/media/series:ro
    restart: unless-stopped
```

```sh
mkdir -p data && sudo chown 1000:1000 data
docker compose up -d
docker compose logs -f aether
```

Un exemple commenté se trouve dans [`docker-compose.example.yml`](../docker-compose.example.yml).

### Accélération matérielle

**Intel (QSV / VAAPI) et AMD (VAAPI)** : exposez `/dev/dri` et ajoutez le groupe qui en est propriétaire (son numéro varie selon les distributions) :

```sh
getent group render | cut -d: -f3     # par exemple 993
```

```yaml
    devices:
      - /dev/dri:/dev/dri
    group_add:
      - "993"                          # groupe « render » de l'hôte
    environment:
      AETHER_HWACCEL: vaapi            # ou qsv (Intel), auto
```

Les pilotes (Intel iHD, Mesa) sont fournis par jellyfin-ffmpeg dans l'image. Vérification : `docker exec aether ffmpeg -hide_banner -init_hw_device vaapi=va:/dev/dri/renderD128 -f lavfi -i nullsrc -t 1 -f null -`.

**NVIDIA (NVENC)** : installez [nvidia-container-toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html) sur l'hôte, puis :

```yaml
    environment:
      AETHER_HWACCEL: nvenc
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu, video]
```

### Agent en Docker

```sh
docker run -d --name aether-agent --network host \
  -v aether-agent:/data -e AETHER_AGENT_DATA_DIR=/data \
  -v /srv/media:/srv/media:ro \
  ghcr.io/fs0ciety7000/aether:latest agent
```

`--network host` est nécessaire pour que le serveur découvre l'agent sur le réseau local (UDP 7360) ; sinon publiez `-p 8097:8097` et saisissez l'adresse à la main.

### Construire l'image

```sh
docker buildx build --platform linux/amd64,linux/arm64 -t aether .
```

---

## Paquets .deb et .rpm

Les paquets installent `/usr/bin/aether`, le compte système `aether`, les dossiers `/var/lib/aether` et `/var/lib/aether-agent`, et deux services systemd : `aether.service` (serveur) et `aether-agent.service` (agent). Ils ne démarrent rien d'eux-mêmes. FFmpeg est recommandé (`Recommends: ffmpeg`).

Debian, Ubuntu, Raspberry Pi OS :

```sh
curl -fLO https://github.com/fs0ciety7000/aether-media/releases/download/v0.1.0/aether_0.1.0_amd64.deb
sudo apt install ./aether_0.1.0_amd64.deb
sudo systemctl enable --now aether
```

Fedora, RHEL, openSUSE :

```sh
sudo dnf install https://github.com/fs0ciety7000/aether-media/releases/download/v0.1.0/aether-0.1.0-1.x86_64.rpm
sudo systemctl enable --now aether
```

(Architectures : `amd64` / `x86_64`, `arm64` / `aarch64`, `armhf` / `armv7hl`.) Sur Fedora, FFmpeg complet vient de RPM Fusion ; `ffmpeg-free` convient pour la lecture sans transcodage des codecs brevetés.

Réglages :

- `/etc/aether/aether.env` et `/etc/aether/agent.env` : variables d'environnement (port, clé TMDB, accélération matérielle, chemin de FFmpeg…) ;
- `/var/lib/aether/aether.toml` : configuration complète (copiez [`aether.example.toml`](../aether.example.toml), aussi installé dans `/usr/share/doc/aether/`).

Pour utiliser jellyfin-ffmpeg ([dépôt Jellyfin](https://jellyfin.org/docs/general/installation/linux)) : `sudo apt install jellyfin-ffmpeg7`, puis dans `/etc/aether/aether.env` :

```sh
AETHER_FFMPEG=/usr/lib/jellyfin-ffmpeg/ffmpeg
AETHER_FFPROBE=/usr/lib/jellyfin-ffmpeg/ffprobe
```

Le service tourne sous `aether`, avec un système de fichiers en lecture seule hors de `/var/lib/aether` : vos médias doivent être **lisibles** par ce compte (`sudo setfacl -R -m u:aether:rX /srv/media`, ou un groupe commun). L'accès GPU est accordé à l'installation (groupes `render` et `video`).

Journaux : `journalctl -u aether -f`.

---

## Homebrew

> La publication du tap n'est pas encore active : en attendant, utilisez le [binaire manuel](#binaire-manuel).

```sh
brew install --cask fs0ciety7000/tap/aether
brew install ffmpeg
aether service install --user       # serveur au démarrage de la session (LaunchAgent)
```

L'agent : `aether agent install --user`. Voir [les commandes `install`](#les-commandes-install-uninstall-status).

---

## Windows

1. Téléchargez `aether_0.1.0_windows_amd64.zip` et extrayez `aether.exe` dans `C:\Program Files\Aether`.
2. Installez FFmpeg (`winget install Gyan.FFmpeg`, ou un build de [jellyfin-ffmpeg](https://github.com/jellyfin/jellyfin-ffmpeg/releases)) et vérifiez que `ffmpeg` est dans le `PATH` du système, ou indiquez son chemin dans `C:\ProgramData\Aether\aether.toml` (`[paths] ffmpeg = "C:\\ffmpeg\\bin\\ffmpeg.exe"`).
3. Dans un terminal **administrateur** :

```powershell
& "C:\Program Files\Aether\aether.exe" service install
New-NetFirewallRule -DisplayName "Aether" -Direction Inbound -Protocol TCP -LocalPort 8096 -Action Allow
```

Le service Windows `aether` démarre automatiquement (démarrage différé) et redémarre en cas d'échec. Données : `C:\ProgramData\Aether` ; journal : `C:\ProgramData\Aether\logs\aether.log`. Gestion : `services.msc`, `aether service status`, `aether service uninstall`.

Pour un essai sans service : `aether.exe serve` (données dans `%LOCALAPPDATA%\Aether`).

---

## Binaire manuel

Linux / macOS :

```sh
curl -fL https://github.com/fs0ciety7000/aether-media/releases/download/v0.1.0/aether_0.1.0_linux_amd64.tar.gz | tar xz aether
sudo install -m 0755 aether /usr/local/bin/aether
aether version
```

Puis, au choix :

- `aether serve` : au premier plan (données dans `~/.local/share/aether`, ou `~/Library/Application Support/Aether` sur macOS) ;
- `sudo aether service install` : service système (systemd sous Linux, LaunchDaemon sous macOS), exécuté sous l'utilisateur qui lance `sudo` (ou `--run-as compte`) ;
- `aether service install --user` : service de votre session (systemd `--user`, LaunchAgent), sans droits administrateur.

Sur macOS, un binaire téléchargé est mis en quarantaine par Gatekeeper : `xattr -d com.apple.quarantine /usr/local/bin/aether`.

### Les commandes `install`, `uninstall`, `status`

```
aether service install|uninstall|status [--user] [--name NOM] [--config F] [--run-as U] [--dry-run]
aether agent   install|uninstall|status [--user] [--method systemd|launchd|cron] [--listen ADR] [--data-dir D]
                                        [--name NOM] [--config F] [--run-as U] [--dry-run]
```

`--dry-run` affiche le fichier d'unité (ou le plist, ou la configuration du service Windows) et les commandes, sans rien modifier. `--name` permet d'installer plusieurs instances. La désinstallation conserve les données.

| | Linux | macOS | Windows |
|---|---|---|---|
| Service système | `/etc/systemd/system/<nom>.service` | `/Library/LaunchDaemons/io.github.fs0ciety7000.<nom>.plist` | service Windows `<nom>` (LocalSystem) |
| `--user` | `~/.config/systemd/user/<nom>.service` | `~/Library/LaunchAgents/…plist` | — |
| Données (système) | `/var/lib/<nom>` | `/Library/Application Support/Aether` (agent : `Aether Agent`) | `C:\ProgramData\Aether` (agent : `Aether Agent`) |
| Données (`--user`) | `~/.local/share/aether`, agent `~/.config/aether-agent` | `~/Library/Application Support/Aether`, agent `…/aether-agent` | — |
| Journaux | `journalctl [--user] -u <nom>` | `/Library/Logs/Aether/<nom>.log` (`~/Library/Logs/…` avec `--user`) | `<données>\logs\<nom>.log` |
| Agent `--method cron` | crontab de l'utilisateur, journal `~/.local/state/aether/agent.log` | idem | — |

Avec `--user` sous Linux, le service s'arrête à la fermeture de la session, sauf après `loginctl enable-linger $USER`.

**Agent sans root ni systemd : `--method cron`.** `aether agent install --user --method cron` ajoute à la crontab de l'utilisateur deux lignes encadrées de commentaires `# BEGIN aether-agent` / `# END aether-agent` : `@reboot` et une surveillance `*/5 * * * *` qui relance l'agent s'il ne tourne pas (verrou `flock` tenu par l'agent pendant toute sa vie, insensible aux PID recyclés). Cette méthode est choisie **d'office** si `systemctl --user` est absent ou en échec (le message l'indique). Journal : `~/.local/state/aether/agent.log` (`$XDG_STATE_HOME` respecté), tourné au-delà de 10 Mio (`agent.log.1`). `uninstall` ne retire que les lignes marquées et arrête l'agent ; `status` indique si l'agent tourne. Les autres lignes de la crontab ne sont jamais modifiées ; une crontab vide ou absente est gérée. `--listen` (ex. `0.0.0.0:23456`) et `--data-dir` sont aussi acceptés par les autres méthodes `--user`.

---

## Installer l'agent sur une machine distante

L'agent rend les médias d'une autre machine (PC, NAS, Mac) visibles par le serveur. Trois commandes, puis l'appairage se fait depuis l'interface.

Linux :

```sh
curl -fL https://github.com/fs0ciety7000/aether-media/releases/download/v0.1.0/aether_0.1.0_linux_amd64.tar.gz | tar xz aether
sudo install -m 0755 aether /usr/local/bin/aether
sudo aether agent install            # ou : aether agent install --user
```

macOS :

```sh
curl -fL https://github.com/fs0ciety7000/aether-media/releases/download/v0.1.0/aether_0.1.0_darwin_arm64.tar.gz | tar xz aether
sudo install -m 0755 aether /usr/local/bin/aether && sudo xattr -d com.apple.quarantine /usr/local/bin/aether
aether agent install --user          # LaunchAgent : accès simple aux dossiers Films, Musique, disques externes
```

Windows (PowerShell administrateur) :

```powershell
Invoke-WebRequest https://github.com/fs0ciety7000/aether-media/releases/download/v0.1.0/aether_0.1.0_windows_amd64.zip -OutFile aether.zip
Expand-Archive aether.zip -DestinationPath "C:\Program Files\Aether" -Force
& "C:\Program Files\Aether\aether.exe" agent install
```

Avec un paquet `.deb` / `.rpm` : `sudo systemctl enable --now aether-agent`.

Ensuite, dans Aether : **Réglages › Bibliothèques › Appairer un agent**. La machine apparaît si elle est sur le même réseau local ; sinon, saisissez `https://<adresse>:8097`. Tapez le code d'appairage affiché par l'agent : dans ses journaux, ou sur `https://127.0.0.1:8097` depuis la machine de l'agent (certificat auto-signé : le navigateur avertit, c'est attendu). Choisissez enfin les dossiers à partager.

L'agent sert **HTTPS par défaut** avec un certificat auto-signé créé au premier démarrage. À l'appairage, le serveur l'épingle et affiche son **empreinte** ; vous pouvez la comparer à celle de l'agent (journaux : `Empreinte du certificat TLS (SHA-256…) : 3F2A 9C01 …`, ou sa page locale). Le code d'appairage ne circule jamais en clair et un intermédiaire qui présenterait un autre certificat fait échouer l'appairage. Si le certificat change ensuite, le serveur refuse l'agent (« Certificat changé ») jusqu'à un **Réappairer**. Les agents appairés en HTTP avant cette version continuent de fonctionner en HTTP ; « Réappairer » les passe en HTTPS. Détails : [AGENT_PROTOCOL.md](AGENT_PROTOCOL.md).

Pare-feu : TCP 8097 (agent) et UDP 7360 (découverte) en entrée sur la machine de l'agent. Derrière un proxy HTTPS qui termine TLS, réglez `tls = false` dans `agent.toml` (ou `AETHER_AGENT_TLS=0`).

---

## Seedbox sans root (ex. Whatbox)

Sur une seedbox mutualisée, on n'a ni `sudo` ni (souvent) `systemd --user`, et les fichiers transitent par Internet. L'agent s'y installe dans votre dossier personnel, démarre par la crontab, et chiffre tout par HTTPS.

1. **Télécharger le binaire** (Linux amd64 dans la plupart des cas ; `uname -m` pour vérifier) :

   ```sh
   mkdir -p ~/bin && cd ~/bin
   curl -fL https://github.com/fs0ciety7000/aether-media/releases/download/v0.1.0/aether_0.1.0_linux_amd64.tar.gz | tar xz aether
   ./aether version
   ```

2. **Choisir un port libre.** Les ports bas et le 8097 sont souvent pris ou interdits sur une machine partagée : prenez un port dans la plage que votre hébergeur autorise (voir son aide), par exemple 23456, et vérifiez qu'il est libre :

   ```sh
   nc -z 127.0.0.1 23456 && echo "occupé" || echo "libre"
   ```

3. **Installer l'agent** (crontab, sans root ; choisie d'office si `systemctl --user` ne répond pas) :

   ```sh
   ~/bin/aether agent install --user --method cron --listen 0.0.0.0:23456 --dry-run   # pour voir
   ~/bin/aether agent install --user --method cron --listen 0.0.0.0:23456
   ~/bin/aether agent status --method cron
   ```

   Données : `~/.config/aether-agent` (ou `--data-dir ~/aether-agent`). Journal : `~/.local/state/aether/agent.log`. L'agent redémarre au reboot et est relancé dans les 5 minutes s'il s'arrête.

4. **Vérifier ffprobe.** L'analyse des fichiers est faite par l'agent, sur la seedbox. Le journal dit `ffprobe=true` au démarrage ; sinon, installez un ffprobe statique dans `~/bin` (la surveillance cron ajoute `~/bin` et `~/.local/bin` au `PATH`) :

   ```sh
   cd ~/bin && curl -fL https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz | tar xJ --strip-components=1 --wildcards '*/ffprobe'
   ./ffprobe -version
   ```

   ou indiquez son chemin dans un `agent.toml` (clé `ffprobe = "/home/moi/bin/ffprobe"`), à passer à l'installation avec `--config ~/agent.toml`. Relancez : `aether agent uninstall --method cron && aether agent install --user --method cron …`.

5. **Appairer.** Dans Aether : Réglages › Bibliothèques › **Appairer un agent** › « Saisir une adresse » : `https://<nom-de-la-seedbox>:23456` (la découverte réseau ne traverse pas Internet). Le code d'appairage est dans le journal :

   ```sh
   grep -E "Code d'appairage|Empreinte" ~/.local/state/aether/agent.log | tail -2
   ```

6. **Vérifier l'empreinte** (facultatif, recommandé à travers Internet) : l'empreinte affichée par Aether après l'appairage doit être identique à la ligne `Empreinte du certificat TLS` du journal. Choisissez ensuite le dossier de vos médias (les racines de navigation se règlent par `browse_roots` dans `agent.toml`, par défaut votre dossier personnel).

**Variante : tunnel SSH.** Si l'hébergeur n'autorise aucun port entrant, faites passer l'agent par SSH depuis la machine du serveur Aether : lancez l'agent en écoute locale (`--listen 127.0.0.1:23456`), puis `ssh -N -L 23456:127.0.0.1:23456 moi@seedbox` (ou `autossh` pour le garder ouvert) et appairez `https://127.0.0.1:23456`. HTTPS et l'épinglage restent actifs de bout en bout.

**Sobriété réseau.** Certains hébergeurs (dont Whatbox) bloquent temporairement une adresse qui ouvre trop de connexions : le serveur ne joint alors plus du tout l'agent (`dial tcp …: i/o timeout`) alors qu'il tourne. Aether reste sobre : une seule connexion HTTP/2 porte tous les échanges avec un agent à jour (mettez à jour l'agent **et** le serveur), au plus 8 connexions en HTTP/1.1, deux analyses à la fois par agent (`agent_concurrency` dans `[library]`), et, après une erreur réseau, l'agent passe hors ligne : un seul essai après 1, 2, 5, 15 puis toutes les 30 minutes, sans que chaque tâche réessaie de son côté. Si l'hébergeur bloque malgré tout, le tunnel SSH ci-dessus fait tout passer par une seule connexion SSH.

**Agent ou simple source SFTP ?** Une source « SFTP / seedbox » ne demande rien sur la seedbox, mais chaque analyse ffprobe et chaque scan lisent les fichiers à travers Internet, et les changements ne sont vus qu'au prochain scan. L'agent analyse les fichiers sur place (seul le résultat JSON voyage), parcourt les dossiers en un seul flux, signale les nouveaux fichiers en direct (SSE) et sert la lecture par plages ; il demande en échange un port ouvert (ou un tunnel) et un processus permanent. Pour une grosse bibliothèque qui change souvent, l'agent ; pour un usage ponctuel ou un hébergeur qui interdit les processus permanents, SFTP.

---

## Mise à jour

Les migrations de la base sont appliquées automatiquement au démarrage. [Sauvegardez](#sauvegarde) avant une mise à jour majeure.

| Installation | Commande |
|---|---|
| Docker Compose | `docker compose pull && docker compose up -d` |
| `.deb` | `sudo apt install ./aether_<version>_amd64.deb` (le service est redémarré) |
| `.rpm` | `sudo dnf install ./aether-<version>-1.x86_64.rpm` |
| Homebrew | `brew upgrade --cask aether`, puis `aether service install --user` (relance le service) |
| Binaire + service (Linux) | remplacez le binaire, puis `sudo systemctl restart aether` (ou `aether-agent`) |
| Binaire + service (macOS) | remplacez le binaire, puis `sudo launchctl kickstart -k system/io.github.fs0ciety7000.aether` |
| Windows | `sc.exe stop aether`, remplacez `aether.exe`, `sc.exe start aether` |

`aether version` affiche la version installée.

---

## Sauvegarde

Tout l'état du serveur est dans son dossier de données (`/data` en Docker, `/var/lib/aether` avec les paquets) :

| Fichier | Contenu | À sauvegarder |
|---|---|---|
| `aether.db` (+ `aether.db-wal`, `aether.db-shm`) | bibliothèques, métadonnées, comptes, progression | **oui** |
| `secret.key` | clé de chiffrement des secrets (jetons Trakt / Simkl, agents) | **oui** — sans elle, les secrets stockés sont perdus |
| `server.id` | identité du serveur auprès des agents | **oui** |
| `aether.toml` | configuration | oui, si présent |
| `cache/` | images, réponses TMDB, transcodages | non (se reconstruit) |

Arrêtez le serveur pendant la copie, pour que la base soit cohérente :

```sh
docker compose stop aether                       # ou : sudo systemctl stop aether
tar czf aether-$(date +%F).tar.gz --exclude=cache -C ./data .
docker compose start aether
```

Restauration : arrêtez le serveur, videz le dossier de données, extrayez l'archive (`tar xzf … -C ./data`), rétablissez le propriétaire (`chown -R 1000:1000 data`, ou `aether:aether` pour les paquets), redémarrez.

L'agent garde dans son dossier `agent-state.json` (partages et serveurs appairés), `agent.token`, et son certificat `agent-tls.crt` / `agent-tls.key` ; les perdre oblige seulement à l'appairer de nouveau.

---

## Désinstallation

- Docker : `docker compose down` (le dossier `./data` reste).
- Paquets : `sudo apt remove aether` / `sudo dnf remove aether`. Les données (`/var/lib/aether*`) et le compte `aether` sont conservés.
- Service installé par `aether … install` : `aether service uninstall` / `aether agent uninstall` (avec `sudo` pour un service système), puis supprimez le binaire et, si besoin, le dossier de données.
