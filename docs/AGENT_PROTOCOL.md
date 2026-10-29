# Protocole de l'Agent Aether

L'agent (`aether agent`) tourne sur une machine qui possède des médias. Il expose des **partages** (dossiers nommés) au serveur Aether principal. Pas de base de données ni d'interface.

## Sécurité
- Authentification : jeton aléatoire (256 bits, base64url) généré au premier démarrage, stocké dans `<data>/agent.token`, affiché dans les logs ; à saisir dans le serveur principal lors de l'ajout de la source.
- En-tête `Authorization: Bearer <jeton>`. Le paramètre `?t=<jeton>` reste accepté sur `/agent/v1/file` (compatibilité), mais le serveur ne donne plus jamais d'URL d'agent à FFmpeg (voir « HTTPS », M5d).
- Comparaison en temps constant ; limitation des échecs (5 / minute / IP).
- Confinement : chaque partage est ouvert avec `os.Root` (Go ≥ 1.24) — aucun accès hors du dossier, liens symboliques sortants refusés. Chemins relatifs, séparés par `/`, sans `..`.
- **HTTPS par défaut** (M5d) : certificat auto-signé épinglé par le serveur, ou `tls_cert`/`tls_key`. `tls = false` pour servir en HTTP derrière un proxy HTTPS ou un VPN. Voir « HTTPS et appairage lié à l'empreinte ».

## Endpoints (préfixe `/agent/v1`)
| Méthode | Chemin | Réponse |
|---|---|---|
| GET | `/info` | `{ "uid", "name", "version", "shares": [{ "name", "path"? }], "ffprobe": bool, "hostname", "tls": bool, "fingerprint"? }` (`path` local omis par défaut) |
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
# tls = false            # HTTP (derrière un proxy HTTPS) ; défaut : HTTPS auto-signé
# http_compat = false    # refuse aussi HTTP pour les appairages antérieurs à HTTPS
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
- `POST /agent/v1/pair` (sans authentification) : **v2 depuis M5d**, le code ne circule plus (voir « Appairage lié à l'empreinte »). Le code est à usage unique ; 5 essais erronés / 10 min / IP puis 429. Le jeton (256 bits) est propre à ce serveur ; seul son hash est stocké.
- `DELETE /agent/v1/pair` (authentifié) : le serveur appelant se désappaire (son jeton est révoqué).

## Navigation et partages (authentifié)
- Racines de navigation : `browse_roots` dans `agent.toml` ou la variable `AETHER_AGENT_BROWSE_ROOTS` (liste séparée comme `PATH`, prioritaire), sinon les emplacements usuels du système (dossier personnel, `/mnt`, `/media`, `/srv`, `/data`, `/volume1`… ; lecteurs sous Windows ; `/Volumes` sous macOS).
- `GET /agent/v1/browse?path=<chemin absolu>` → `{ "path", "parent"?, "dirs": [{ "name", "path" }] }` (sans `path` : les racines). Hors racines → 403 `denied`.
- `GET /agent/v1/shares` → `[{ "name", "path", "managed": bool }]` (`managed` = créé par l'interface, supprimable).
- `POST /agent/v1/shares` `{ "path", "name"? }` → `201 { "name", "path" }` — le dossier doit exister et être dans les racines ; nom dérivé du dossier si absent (minuscules, `[a-z0-9_-]`, suffixe numérique si pris). Si un partage existe déjà pour ce chemin, il est renvoyé (200).
- `DELETE /agent/v1/shares/{name}` → 204 (partages gérés uniquement ; 409 pour ceux du fichier).
- Les partages ajoutés sont immédiatement servis et surveillés (événements).

## Découverte sur le réseau local
- L'agent écoute en UDP sur le port **7360** (`discovery_port`, `0` pour désactiver).
- Requête (diffusion broadcast) : la chaîne `AETHER_DISCOVER_AGENT_V1`.
- Réponse (unicast vers l'émetteur) : JSON `{ "uid", "name", "hostname", "version", "port", "tls": bool, "scheme": "https"|"http", "fingerprint"? }`. Le serveur en déduit l'URL `<scheme>://<adresse source>:<port>`. L'empreinte annoncée en UDP n'est pas authentifiée : affichage seulement, l'appairage vérifie la vraie.
- En Docker, la découverte exige le réseau `host` ; sinon on saisit l'adresse à la main.

---

# HTTPS et appairage lié à l'empreinte (M5d)

Objectif : l'agent peut être joint **à travers Internet** (seedbox, VPS) sans VPN ni proxy, sans autorité de certification, et sans qu'un intermédiaire (MITM) puisse lire les fichiers ou se faire passer pour l'agent.

## Certificat de l'agent
- Au premier démarrage, l'agent crée `<data>/agent-tls.key` et `<data>/agent-tls.crt` (0600) : ECDSA P-256, validité 20 ans, SAN = nom d'hôte, `localhost`, 127.0.0.1, ::1 et les adresses IP de la machine (utile seulement aux navigateurs : le serveur ne vérifie ni la chaîne ni le nom).
- **Empreinte** = SHA-256 du certificat DER. Forme canonique : 64 caractères hexadécimaux minuscules (API, base). Forme affichée : 16 groupes de 4 (`3F2A 9C01 …`), dans les journaux au démarrage (`Empreinte du certificat TLS (SHA-256, auto-signé) : …`), sur la page locale `GET /` et dans `GET /agent/v1/local/status` (`tls`, `fingerprint`, `certSource`, `httpCompat`). Le serveur accepte aussi les formes `AB:CD:…` et `sha256:…`.
- Régénérer le certificat : arrêter l'agent, supprimer les deux fichiers, relancer, puis **réappairer** sur chaque serveur.
- `tls_cert` / `tls_key` : certificat fourni (origine `custom`). S'il est reconnu par une autorité (Let's Encrypt…), le serveur le vérifie comme un site web (pas d'épinglage : les renouvellements ne cassent rien) ; sinon il l'épingle.
- `tls = false` (ou `AETHER_AGENT_TLS=0`) : HTTP seul, pour un proxy HTTPS qui termine TLS devant l'agent, ou un VPN.

## Un seul port, HTTPS et HTTP
L'agent sert HTTPS et HTTP **sur le même port** : le premier octet d'une connexion TLS (enregistrement handshake, `0x16`) ne peut pas commencer une requête HTTP. En HTTP, seuls restent permis :
- la page d'état locale (`GET /`, `/agent/v1/local/status`, clients loopback uniquement) ;
- les appels authentifiés des **appairages antérieurs à HTTPS** (compatibilité, ci-dessous).

Tout le reste répond `426 { "error": "tls_required" }`. L'appairage n'est jamais accepté en HTTP quand l'agent sert HTTPS.

En HTTPS, l'agent annonce `h2` puis `http/1.1` par ALPN : un serveur à jour négocie **HTTP/2** et multiplexe tous ses appels sur une seule connexion (voir « Connexions »). Les agents antérieurs n'annoncent que `http/1.1` et restent compatibles. HTTP en clair reste en HTTP/1.1.

## Épinglage côté serveur
- `remote_agents.fingerprint` (migration 00011) : empreinte épinglée à l'appairage ; vide = HTTP (appairage antérieur, ou `tls = false`) ou HTTPS vérifié par les autorités du système. L'API l'expose avec `transport` : `https-pinned`, `https-ca` ou `http`.
- Tous les appels du serveur à un agent passent par un client HTTP dont la configuration TLS n'utilise aucune autorité : `VerifyConnection` (préféré à `VerifyPeerCertificate`, car il s'exécute aussi sur les sessions TLS reprises) compare le SHA-256 du certificat présenté à l'empreinte stockée. Cela couvre l'API (infos, navigation, partages), la lecture par plages (`Range`), le parcours NDJSON, le flux SSE des changements, `probe` et la surveillance (ping). Les clients sont partagés par agent et par empreinte (un seul pool de connexions par agent, voir « Connexions »).
- Un agent épinglé n'est jamais joint en `http://` (refus explicite).
- **Changement de certificat après l'appairage** : chaque appel échoue (`FingerprintError`), l'agent passe au statut `cert_mismatch` (événement SSE `agent.status`), les appels d'API proxifiés répondent `502 agent_cert_mismatch`, les sources passent hors ligne avec le message. Seul un **réappairage** (nouveau code) enregistre le nouveau certificat.
- **FFmpeg / ffprobe ne se connectent jamais à l'agent** : `InputURL` d'une source agent est vide, le serveur leur donne une URL signée et expirante de son proxy interne loopback `/internal/vfs/…`, qui lit le fichier à travers le client épinglé. Bénéfice annexe : le jeton n'apparaît plus dans la ligne de commande de FFmpeg. L'analyse des fichiers reste faite par l'agent (`POST /probe`). Le proxy accepte aussi un client dont l'adresse est celle sur laquelle il a été joint (serveur lié à une seule adresse du LAN).

## Appairage lié à l'empreinte (v2)
Notations : `code` = code normalisé (8 caractères, majuscules, sans tiret) ; `F` = empreinte vue par le serveur (`""` en HTTP) ; `N` = 16 octets aléatoires du serveur ; `‖` = séparateur `0x00`.

1. Le serveur ouvre une connexion TLS sans faire confiance à rien et relève `F`. Si l'adresse saisie est `http://`, il essaie d'abord HTTPS sur le même port (rien n'est envoyé en clair si l'agent parle HTTPS).
2. `K = Argon2id(code, sel = "aether-pair-v2|" ‖ N, t=3, m=32 Mio, p=1, 32 octets)`.
3. `POST /agent/v1/pair` **sur une connexion épinglée à `F`** :
   `{ "version": 2, "serverId", "serverName", "fingerprint": F, "nonce": b64url(N), "proof": b64url(HMAC-SHA256(K, "aether-pair-v2"‖"server"‖F‖hex(N)‖serverId)) }`
4. L'agent (HTTPS) : refuse une requête arrivée en HTTP (426) ; si `F` ≠ sa propre empreinte → **invalide le code**, compte un échec, `409 fingerprint_mismatch` ; sinon recalcule `K` avec son code courant et vérifie la preuve en temps constant → `401 invalid_code` en cas d'échec (le code reste valable ; 5 essais / 10 min / IP). Une seule dérivation Argon2id à la fois.
5. Succès : le code est consommé, un jeton est émis et l'agent répond
   `{ "token", "uid", "name", "version", "pairVersion": 2, "fingerprint", "certSource", "proof": b64url(HMAC-SHA256(K, "aether-pair-v2"‖"agent"‖F‖hex(N)‖token)) }`.
6. Le serveur vérifie la preuve de l'agent (un faux agent ne connaît pas le code), vérifie que `fingerprint` = `F`, puis épingle `F` (sauf certificat `custom` reconnu par une autorité).

Cas particuliers :
- Agent `tls = false` derrière un proxy HTTPS : l'agent ne peut pas savoir quel certificat le serveur a vu ; il vérifie la preuve avec le `F` annoncé. Le serveur n'accepte alors l'appairage que si le certificat du proxy est **reconnu par une autorité** (sinon `agent_cert_untrusted`, et le jeton tout juste émis est révoqué).
- Agent `tls = false` en HTTP pur : appairage accepté, avec un avertissement (le code ne circule toujours pas en clair).
- Serveur ancien (v1, `{ "code" }` en clair) : `400 pairing_upgrade_required`, et le code, exposé, est invalidé. Agent ancien face à un serveur récent : `401`, le serveur affiche « code incorrect ou agent trop ancien ».
- Réappairer (`POST /api/v1/agents/pair { url, code, agentId }`) met à jour la même ligne (même `uid`, ou ligne `legacy:` héritée de M5b) ; une autre machine est refusée (`agent_not_same`).

## Modèle de menace
| Attaquant | Ce qu'il obtient |
|---|---|
| Écoute passive (Wi-Fi public, FAI, hébergeur) | Rien après l'appairage (TLS). Pendant l'appairage : `F`, `N` et la preuve, qui ne permettent qu'une attaque hors ligne sur le code (ci-dessous). |
| Intermédiaire actif **pendant l'appairage**, qui relaie avec son propre certificat | L'agent voit `F` ≠ son empreinte : refus, **code invalidé** (le relais ne peut plus servir). Le serveur affiche `agent_fingerprint_mismatch`. |
| Intermédiaire actif qui ne relaie pas et répond lui-même | Il ne peut pas produire la preuve de l'agent sans le code : `agent_proof_invalid`, rien n'est enregistré. Pour réussir, il doit retrouver le code hors ligne **puis** l'utiliser auprès du vrai agent avant son expiration (10 min) et avant l'appairage légitime. |
| Intermédiaire actif **après l'appairage** | Certificat différent : tout est refusé (`cert_mismatch`), rien n'est lu ni écrit. |
| Devinette en ligne du code | 5 essais / 10 min / IP (/64 en IPv6) sur 31⁸ ≈ 8,5·10¹¹ codes. |
| Découverte UDP falsifiée | Peut annoncer une fausse adresse ou empreinte (affichage seul) ; l'appairage vers cette adresse échoue comme ci-dessus. |

**Attaque hors ligne sur le code.** Un enregistrement de l'échange permet de tester des codes : chaque essai coûte une dérivation Argon2id (32 Mio, 3 passes), salée par le nonce du serveur (aucun précalcul possible). Même à 10⁵ essais/s (grappe de GPU, estimation généreuse pour une fonction à mémoire dure), parcourir 31⁸ codes demande ~100 jours, contre une fenêtre utile de 10 minutes : le code est à usage unique, expire, et un relais l'invalide. Un simple HMAC (~10¹⁰ essais/s sur GPU) aurait été cassé en quelques minutes, d'où Argon2id. Un PAKE (SPAKE2, CPace) supprimerait toute attaque hors ligne ; écarté faute d'implémentation de référence dans la bibliothèque standard Go (suite T-0BD).

Hors périmètre : une machine (agent ou serveur) compromise ; le vol de `agent-tls.key` (permet d'usurper l'agent : régénérer et réappairer) ; le déni de service (un intermédiaire peut toujours bloquer les échanges ou faire tourner le code).

## Compatibilité avec les appairages antérieurs (HTTP)
- Au premier démarrage d'un agent ≥ M5d, un état sans champ `version` vient d'un agent qui ne parlait qu'HTTP : chaque serveur déjà appairé est marqué `plain` et **continue de fonctionner en HTTP** sur le même port ; idem pour le jeton manuel `agent.token` s'il existait (`legacyTokenPlain`). L'agent l'écrit dans ses journaux au démarrage et au premier appel HTTP de chaque appelant ; la page locale marque ces serveurs « HTTP ».
- Côté serveur, ces agents gardent leur URL `http://` (`transport: "http"`) et Réglages › Agents propose **Réappairer**, qui passe en HTTPS épinglé : le drapeau `plain` de ce serveur est effacé et l'agent refuse ensuite HTTP pour lui.
- `http_compat = false` interdit toute requête HTTP (sauf la page locale). Supprimer `agent.token` en génère un nouveau, jamais accepté en HTTP.
- **Jeton manuel** (source « agent » avec `{ url, token, share }`) : `fingerprint` facultatif (`AB12 CD34 …`, `ab:12:…`). Absent avec une adresse `https://`, le certificat vu lors du test de connexion est épinglé à la création (**TOFU**, `warning: "fingerprint_tofu"` dans la réponse du test, avertissement dans les journaux), sauf s'il est reconnu par une autorité. Une modification de la source qui garde la même adresse garde l'empreinte (pas de nouveau TOFU).

# Connexions : sobriété envers un agent distant

Un agent est souvent une seedbox jointe à travers Internet, chez un hébergeur dont la protection anti-abus peut bloquer une adresse qui ouvre beaucoup de connexions TCP/TLS (symptôme : `dial tcp …: i/o timeout` depuis le serveur, alors que l'agent répond en local). Le serveur s'impose donc :

- **Un transport HTTP par agent** (adresse + empreinte), quel que soit l'appel : API, `probe`, parcours NDJSON, flux SSE, ping, lecture par plages. Les corps de réponse sont lus jusqu'au bout (au plus 64 Kio) puis fermés, pour que la connexion serve à nouveau.
- **HTTP/2** quand l'agent le propose (agents à jour, HTTPS) : une seule connexion TCP, tous les appels multiplexés dessus ; abandonner une lecture ne fait que réinitialiser son flux. Après une période d'inactivité, la première requête part seule jusqu'à obtenir sa connexion, pour éviter une rafale de poignées de main avant que HTTP/2 soit connu. Une connexion silencieuse est sondée (PING HTTP/2 après 30 s) et abandonnée si elle ne répond plus.
- **HTTP/1.1** sinon (agent ancien, `tls = false`) : keep-alive, au plus **8 connexions** par agent (`MaxConnsPerHost`, inactives comprises), dont une tenue en permanence par le flux SSE. La lecture par plages demande des fenêtres bornées et croissantes (256 Kio, 1 Mio, 4 Mio… jusqu'à 64 Mio, `Range: bytes=a-b`) au lieu d'une plage ouverte : une fenêtre lue jusqu'au bout, ou abandonnée à moins de 256 Kio de sa fin (le reste est lu), garde sa connexion.
- **Concurrence par agent** : au plus `library.agent_concurrency` (défaut 2) analyses `probe` et parcours de scan simultanés par agent, indépendamment de `library.probe_workers` ; les agents ne se bloquent pas entre eux.
- **Disjoncteur par agent** : à la première erreur réseau (connexion refusée, réinitialisée, délai de connexion ou de poignée TLS dépassé), l'agent passe **hors ligne** (statut de l'agent et de ses sources, événements `agent.status` / `source.status`), ses connexions inactives sont fermées et plus aucun appel ne part : ils échouent aussitôt (`vfs.ErrOffline`), les analyses `media.probe` sont **différées** sans consommer d'essai (`jobs.Postpone`), le ping périodique saute l'agent. Un **seul** `GET /agent/v1/info` de sonde est envoyé après 1 min, puis 2, 5, 15 et 30 min (plafond) ; dès qu'il aboutit (même avec une erreur HTTP : l'agent est joignable), l'agent repasse en ligne et les analyses différées reprennent. Une réponse HTTP de l'agent (4xx/5xx), un certificat différent (`cert_mismatch`) ou l'abandon par l'appelant n'ouvrent pas le disjoncteur. Un appairage réussi le referme.
- Le flux SSE garde sa propre reconnexion (1 s à 60 s, exponentielle) ; FFmpeg ne joint jamais l'agent (proxy loopback, une requête par plage, toutes portées par le transport ci-dessus).
- **Images clés d'un remux** : le serveur ne fait jamais lire tout un fichier distant par ffprobe. Pour un MKV/WebM, il lit l'index Cues par quelques requêtes `Range: bytes=a-b` bornées (en-tête, SeekHead, Cues, trois échantillons de clusters de 2 Mio pour vérifier que l'index est complet : ~6 Mo pour un remux Blu-ray), puis garde la liste en base (`media_keyframes`). Sans index exploitable (TS, MP4, MKV sans Cues), la vidéo est transcodée au lieu d'être copiée. Les fichiers locaux peuvent encore être analysés par ffprobe (40 s au plus pendant le démarrage, puis en arrière-plan pour la lecture suivante).
