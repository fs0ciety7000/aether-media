# Protocole de l'Agent Aether (v1)

L'agent (`aether agent`) tourne sur une machine qui possède des médias. Il expose des **partages** (dossiers nommés) au serveur Aether principal. Pas de base de données ni d'interface.

## Sécurité
- Authentification : jeton aléatoire (256 bits, base64url) généré au premier démarrage, stocké dans `<data>/agent.token`, affiché dans les logs ; à saisir dans le serveur principal lors de l'ajout de la source.
- En-tête `Authorization: Bearer <jeton>` ; pour les URL données à FFmpeg, paramètre `?t=<jeton>` accepté sur `/agent/v1/file` uniquement.
- Comparaison en temps constant ; limitation des échecs (5 / minute / IP).
- Confinement : chaque partage est ouvert avec `os.Root` (Go ≥ 1.24) — aucun accès hors du dossier, liens symboliques sortants refusés. Chemins relatifs, séparés par `/`, sans `..`.
- TLS optionnel (`tls_cert`, `tls_key`) ; sinon à utiliser derrière un VPN (WireGuard, Tailscale) ou un proxy HTTPS.

## Endpoints (préfixe `/agent/v1`)
| Méthode | Chemin | Réponse |
|---|---|---|
| GET | `/info` | `{ "version", "shares": [{ "name", "path"? }], "ffprobe": bool, "hostname" }` (`path` local omis par défaut) |
| GET | `/walk?share=&path=` | NDJSON en flux : une ligne `Entry` par fichier/dossier (récursif, parents d'abord) ; dernière ligne `{"done":true}` ou `{"error":"..."}` |
| GET | `/stat?share=&path=` | `Entry` · 404 `{"error":"not_found"}` |
| GET | `/readdir?share=&path=` | `Entry[]` |
| GET/HEAD | `/file?share=&path=` | contenu brut, `Range` (206), `Last-Modified`, `Content-Length`, `Accept-Ranges: bytes` |
| POST | `/probe?share=&path=` | sortie JSON brute de `ffprobe -show_format -show_streams -show_chapters` (exécuté localement) · 422 si illisible |
| GET | `/events` | SSE : `event: change` `data: {"share","paths":[...]}` (regroupé, anti-rebond 10 s) ; `: ping` toutes les 25 s |

`Entry` : `{ "path": "Films/Heat (1995)/heat.mkv", "size": 123, "mtime": "2026-10-28T10:00:00Z", "dir": false }` (chemin relatif au partage).

Erreurs : `{ "error": "<code>", "message": "..." }` avec 400 (chemin invalide), 401, 404, 429, 500.

## Configuration de l'agent (`agent.toml`)
```toml
listen = "0.0.0.0:8097"
# tls_cert = "/etc/aether/cert.pem"
# tls_key  = "/etc/aether/key.pem"
[[share]]
name = "films"
path = "/srv/media/films"
[[share]]
name = "series"
path = "/srv/media/series"
```

---

# v2 — Appairage sans configuration (M5c)

Objectif : comme Jellyfin, on installe l'agent, on le lance **sans fichier de configuration**, et tout le reste se fait depuis l'interface du serveur : découverte sur le réseau, appairage par code, choix des dossiers à partager. `agent.toml` reste possible (avancé) et se combine avec l'état géré par l'interface.

## Démarrage sans configuration
- `aether agent` sans `-config` : écoute `0.0.0.0:8097`, données dans le dossier par défaut, aucun partage.
- État persistant `<data>/agent-state.json` (0600) : `{ "uid", "name", "shares": [{ "name", "path" }], "servers": [{ "id", "name", "tokenHash", "pairedAt" }] }`. Les partages de `agent.toml` et de l'état s'additionnent (noms uniques ; ceux du fichier ne sont pas supprimables depuis l'interface).
- `name` : nom d'affichage, par défaut le nom de la machine.
- Le jeton historique `agent.token` reste accepté (appairage manuel).

## Code d'appairage
- Un code de 8 caractères (alphabet sans ambiguïté `ABCDEFGHJKMNPQRSTUVWXYZ23456789`, affiché `XXXX-XXXX`) est généré au démarrage puis renouvelé toutes les 10 minutes et après chaque appairage réussi.
- Il est affiché dans les logs (`Code d'appairage : K7PQ-4XRM (valable 10 min)`) et sur la page locale de l'agent `GET /` — **servie uniquement aux clients locaux (loopback)** : nom, adresses, code, partages, serveurs appairés.
- `POST /agent/v1/pair` (sans authentification) `{ "code", "serverName", "serverId" }` → `{ "token", "uid", "name", "version" }`. Le code est à usage unique ; 5 essais erronés / 10 min / IP puis 429 ; comparaison en temps constant. Le jeton (256 bits) est propre à ce serveur ; seul son hash est stocké.
- `DELETE /agent/v1/pair` (authentifié) : le serveur appelant se désappaire (son jeton est révoqué).

## Navigation et partages (authentifié)
- Racines de navigation : `browse_roots` dans `agent.toml`, sinon les emplacements usuels du système (dossier personnel, `/mnt`, `/media`, `/srv`, `/data`, `/volume1`… ; lecteurs sous Windows ; `/Volumes` sous macOS).
- `GET /agent/v1/browse?path=<chemin absolu>` → `{ "path", "parent"?, "dirs": [{ "name", "path" }] }` (sans `path` : les racines). Hors racines → 403 `denied`.
- `GET /agent/v1/shares` → `[{ "name", "path", "managed": bool }]` (`managed` = créé par l'interface, supprimable).
- `POST /agent/v1/shares` `{ "path", "name"? }` → `201 { "name", "path" }` — le dossier doit exister et être dans les racines ; nom dérivé du dossier si absent (minuscules, `[a-z0-9_-]`, suffixe numérique si pris). Si un partage existe déjà pour ce chemin, il est renvoyé (200).
- `DELETE /agent/v1/shares/{name}` → 204 (partages gérés uniquement ; 409 pour ceux du fichier).
- Les partages ajoutés sont immédiatement servis et surveillés (événements).

## Découverte sur le réseau local
- L'agent écoute en UDP sur le port **7360** (`discovery_port`, `0` pour désactiver).
- Requête (diffusion broadcast) : la chaîne `AETHER_DISCOVER_AGENT_V1`.
- Réponse (unicast vers l'émetteur) : JSON `{ "uid", "name", "hostname", "version", "port", "tls": bool }`. Le serveur en déduit l'URL `http(s)://<adresse source>:<port>`.
- En Docker, la découverte exige le réseau `host` ; sinon on saisit l'adresse à la main.
