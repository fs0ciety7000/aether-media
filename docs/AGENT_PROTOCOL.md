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
