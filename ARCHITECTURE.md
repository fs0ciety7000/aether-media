# Aether — Architecture (draft v0.1)

> Statut : **stack validée** (Go + SQLite + SvelteKit, 2026-10-27).
> Ce document s'inspire de l'étude de Jellyfin (fork `fs0ciety7000/jellyfin`, commit `02c2951`). Aucun code n'est repris : on garde les idées et on les réécrit plus simplement.

---

## 1. Étude de Jellyfin — ce qu'on retient

| Domaine | Dans Jellyfin | Ce qu'Aether en fait |
|---|---|---|
| **Parsing des noms** | `Emby.Naming` : parseurs isolés (Video, TV, Audio), règles pour les extras, le « stacking » (`CD1/CD2`), le 3D, les dates. Testable sans I/O. | Package `naming` **pur** (sans accès disque, sans DB) avec des tests sur un large corpus de noms de fichiers (conventions Sonarr/Radarr/Lidarr). |
| **Résolution / scan** | `LibraryManager` + chaîne d'`IItemResolver` (Movies, TV, Audio, Photo…) + règles d'ignore (`.ignore`, patterns). Un arbre `BaseItem` (Folder → Series → Season → Episode). | Même principe : un **walker** + des **resolvers** par type de bibliothèque, qui produisent un arbre d'items. Règles d'ignore (`.aetherignore`, `.ignore`, fichiers samples/extras). |
| **Métadonnées** | `MediaBrowser.Providers` : un `ProviderManager` qui orchestre des fournisseurs locaux (NFO via `XbmcMetadata`) et distants (TMDB, TVDB, MusicBrainz…), un ordre de priorité, une fusion des champs, la notion de champs verrouillés. | Interface `Provider` (Search / Fetch / Images), ordre configurable par bibliothèque, **NFO local prioritaire**, champs verrouillables, cache HTTP et respect des quotas (rate limit). |
| **Probe & encodage** | `MediaBrowser.MediaEncoding` : ffprobe → `ProbeResultNormalizer`, `TranscodeManager`, HLS dynamique (`DynamicHlsController`), extraction de keyframes, subtitles. | Package `media/probe` (ffprobe JSON → modèle normalisé), package `stream` : décision direct play / remux / transcode, **HLS fMP4** à la demande, gestion du cycle de vie des processus FFmpeg. |
| **Données utilisateur** | `UserDataManager` : position, vu/non vu, favoris, note par (user, item). « Next Up », « Resume ». | Table `user_item_data`. Les vues « Reprendre » et « À suivre » sont calculées en SQL. |
| **API** | Jellyfin.Api : ~60 contrôleurs REST et WebSocket de session. | API REST **beaucoup plus petite**, versionnée (`/api/v1`), décrite par OpenAPI, plus un canal d'événements (SSE ou WebSocket). |
| **Tâches** | `ScheduledTasks` : scan, extraction de chapitres, trickplay, nettoyage. | Un **job runner** interne avec file persistante, priorités, progression visible dans l'UI. |

### Ce qu'on évite volontairement
- **Le modèle « BaseItem géant »** (une table aux dizaines de colonnes, avec beaucoup de colonnes nulles selon le type). Aether garde une table `items` commune et légère, plus des tables d'extension par type quand c'est utile.
- **L'héritage Emby / la double couche** `Emby.*` / `MediaBrowser.*` / `Jellyfin.*` : un seul module clair par domaine.
- **Une surface d'API gigantesque** : on expose seulement ce dont le client web a besoin. Compatibilité avec les clients Jellyfin : **hors périmètre** de la v1.
- **Le plugin system dès le départ** : on prévoit des interfaces internes propres (providers) et on n'ouvre de plugins externes qu'après la v1.

---

## 2. Stack proposée (à valider)

### Recommandation : **Go** pour le backend

| Critère | Go | Rust |
|---|---|---|
| Vitesse de développement | ✅ très bonne, compilation rapide | ⚠️ plus lente (compilation, borrow checker) |
| Binaire unique cross-plateforme | ✅ trivial (`GOOS/GOARCH`), et sans CGO si on utilise SQLite en pur Go | ✅ possible, mais la cross-compilation est plus pénible (cibles C) |
| Concurrence (scan, jobs, streams) | ✅ goroutines et context, naturel | ✅ tokio, puissant mais plus verbeux |
| Performance | ✅ largement suffisante : le travail lourd est fait par FFmpeg | ✅ supérieure, mais on n'en profite peu ici |
| Écosystème (HTTP, SQLite, fsnotify) | ✅ mature | ✅ mature |
| Contributeurs open source | ✅ plus accessible | ⚠️ barrière plus haute |

**Conclusion** : le goulot d'étranglement d'un serveur multimédia, c'est l'I/O disque et FFmpeg, pas le CPU du serveur applicatif. Go donne le meilleur ratio vélocité / simplicité / portabilité. Rust reste l'alternative si tu préfères la rigueur du typage et la mémoire maîtrisée.

### Détail
| Couche | Choix proposé | Pourquoi |
|---|---|---|
| Langage | **Go 1.26** | voir ci-dessus |
| HTTP | `net/http` (routeur 1.22+) ou **chi** | standard, léger, pas de framework lourd |
| Base de données | **SQLite** (mode WAL) via `modernc.org/sqlite` (pur Go, pas de CGO) + **FTS5** pour la recherche | zéro dépendance externe pour l'utilisateur ; Postgres en option plus tard |
| Accès aux données | **sqlc** (SQL typé et généré) + migrations `goose` | du SQL explicite, pas d'ORM magique |
| Surveillance FS | `fsnotify` + scan périodique de secours | les NAS/SMB ne remontent pas toujours les événements |
| Média | **FFmpeg / ffprobe** externes (binaire système ou embarqué dans les paquets) | référence absolue ; on ne lie pas libav |
| Images | redimensionnement en Go (`x/image/draw`) + cache disque WebP/AVIF ; frames vidéo via FFmpeg | pas de dépendance native |
| Auth | sessions et tokens opaques en DB, mots de passe en **argon2id** | simple et sûr, pas de JWT nécessaire |
| Config | fichier TOML + variables d'environnement + UI admin | compatible Docker |
| Frontend | **SvelteKit** (adapter-static, SPA) + **TypeScript** | bundle léger, réactivité fine, transitions natives, très adapté à une UI soignée |
| Styles | **CSS natif + tokens CSS custom properties** (pas de framework utilitaire imposé) | le design system *est* les tokens ; thèmes = jeux de variables |
| Lecteur | `<video>` natif + **hls.js** (quand le HLS natif est absent) | le lecteur est entièrement maîtrisé et stylé |
| PWA | manifest + service worker (shell hors-ligne, pas de médias) | « installable » sur desktop et mobile |
| Distribution | binaire unique (le web est embarqué via `embed.FS`), **Docker** (image avec FFmpeg), paquets `.deb`/`.rpm`, Homebrew, installeur Windows (plus tard) | Linux / macOS / Windows |
| Qualité | `golangci-lint`, `go test`, Vitest, Playwright (e2e), CI GitHub Actions | |

---

## 3. Vue d'ensemble

```
┌─────────────────────────── aether (binaire unique) ───────────────────────────┐
│                                                                               │
│  web/ (SvelteKit SPA, embarquée)   ──►  /             fichiers statiques      │
│                                                                               │
│  api  (REST /api/v1 + SSE /api/v1/events)                                     │
│   │                                                                           │
│   ├── auth ── users ── userdata (progression, vu, favoris)                    │
│   ├── library ──┬── scanner (walker + resolvers) ◄── naming (pur)             │
│   │             └── watcher (fsnotify)                                        │
│   ├── metadata ── providers: nfo | tmdb | tvdb | musicbrainz | fanart | local │
│   ├── images ── cache (resize, blurhash)                                      │
│   ├── media/probe ── ffprobe                                                  │
│   ├── stream ── decision ── directplay | remux-HLS | transcode-HLS ── ffmpeg  │
│   ├── collections / playlists                                                 │
│   ├── search (FTS5)                                                           │
│   └── jobs (file persistante, workers, progression)                           │
│                                                                               │
│  store (SQLite WAL, sqlc)        data dir : db, cache images, transcodes      │
└───────────────────────────────────────────────────────────────────────────────┘
```

### Arborescence du code (prévue)
```
cmd/aether/            point d'entrée (serve, scan, user add, migrate…)
internal/
  config/              chargement TOML / env
  store/               migrations, requêtes sqlc, transactions
  naming/              parseurs de noms de fichiers (pur, très testé)
  library/             bibliothèques, walker, resolvers, watcher
  media/probe/         ffprobe → MediaInfo normalisé
  metadata/            orchestrateur + providers/*
  images/              téléchargement, cache, redimensionnement, blurhash
  stream/              profils clients, décision, sessions HLS, ffmpeg runner
  users/ auth/         comptes, sessions, permissions, contrôle parental
  userdata/            progression, vu, favoris, reprendre / à suivre
  collections/         collections manuelles et automatiques (TMDB), playlists
  search/              FTS5, filtres et facettes
  jobs/                scheduler, file, workers
  api/                 handlers HTTP, DTO, OpenAPI, SSE
web/                   SvelteKit
docs/                  documentation
```

---

## 4. Modèle de données (première ébauche)

```
libraries(id, name, kind[movies|shows|music|photos|mixed], settings_json, created_at)
library_paths(id, library_id, path)

items(id, library_id, parent_id, kind, title, sort_title, original_title, year,
      overview, path, size, mtime, fingerprint, runtime_ms, premiere_date,
      index_number, parent_index_number, official_rating, community_rating,
      provider_ids_json, locked_fields_json, date_added, date_updated, missing)
  kind ∈ movie | series | season | episode | album | artist | track | photo | photo_album | folder
  -- arbre via parent_id ; les épisodes pointent vers la saison, qui pointe vers la série

media_files(id, item_id, path, container, size, bitrate, duration_ms, probed_at)  -- multi-versions
media_streams(id, media_file_id, index, type[video|audio|subtitle|attachment],
              codec, profile, level, width, height, hdr, channels, language, title,
              is_default, is_forced, is_external, external_path)
chapters(media_file_id, index, start_ms, title, image_path)

images(item_id, type[poster|backdrop|logo|thumb|banner], path_or_url, width, height,
       blurhash, dominant_color, source)
people(id, name, provider_ids_json, image)   item_people(item_id, person_id, role, kind, order)
genres / studios / tags  + tables de liaison

users(id, name, password_hash, is_admin, parental_max_rating, prefs_json, created_at)
user_library_access(user_id, library_id)
sessions(token_hash, user_id, device_name, client, last_seen, created_at)
user_item_data(user_id, item_id, position_ms, played, play_count, last_played_at,
               is_favorite, rating, audio_stream_idx, subtitle_stream_idx)

collections(id, user_id NULL=partagée, name, kind[manual|smart|tmdb], rules_json)
collection_items(collection_id, item_id, order)
jobs(id, kind, payload_json, status, progress, error, run_after, created_at)
items_fts (FTS5 : title, original_title, overview, people, genres)
```

---

## 5. Pipelines clés

### 5.1 Scan
> Implémenté en M2 : `internal/naming` (parseurs purs), `internal/library` (walker, resolvers, scanner, watcher, job de probe), `internal/jobs` (file SQLite), `internal/events` (SSE). Identité stable des items via `items.key` (`movie:<dossier>|<titre>|<année>|<édition>`, `series:<dossier>`, `season:<dossier>#<n>`, `episode:<fichier>`, `artist:<lib>:<nom>`, `album:<dossier>|<titre>`, `track:<fichier>`). Mesuré : 2000 fichiers → premier scan 0,7 s, rescan inchangé 40 ms.

1. **Walk** des chemins de la bibliothèque, application des règles d'ignore.
2. **Fingerprint** rapide (taille + mtime + inode, avec un hash partiel en option) : on ne retraite que ce qui a changé.
3. **Resolve** : `naming` parse le chemin (titre, année, SxxEyy, multi-épisodes, date, disque, edition `{edition-…}`, ids `{tmdb-123}` / `[imdbid-tt…]` façon *arr) et le resolver construit ou met à jour l'arbre.
4. **Upsert** en transaction ; les items disparus sont marqués `missing` (période de grâce pour les disques réseau démontés) avant suppression.
5. Enfile des jobs : `probe` → `metadata` → `images` → (plus tard) `chapters`/`trickplay`.
6. Événements SSE (`library.progress`, `item.added`) : l'UI se met à jour en direct.

### 5.1 bis — Accès à la base (T-089)
- `store.Store` ouvre deux pools sur le même fichier SQLite (WAL, `busy_timeout` 15 s, `_txlock=immediate`) : le pool principal pour les lectures et les écritures d'une seule instruction, et un **pool d'écriture d'une seule connexion** pour `Store.Tx`. Les transactions démarrent en `BEGIN IMMEDIATE` et s'enchaînent dans l'ordre d'arrivée, au lieu d'échouer quand une transaction `DEFERRED` qui avait lu tente d'écrire après le commit d'une autre (`SQLITE_BUSY_SNAPSHOT`, que `busy_timeout` ne couvre pas).
- Le corps d'une transaction n'écrit que par le `q` reçu (une écriture par le pool principal attendrait le verrou qu'il détient), et le travail lent (réseau, ffprobe) se fait avant `Tx`.
- Les agrégats coûteux (facettes de filtres, genres de l'accueil) sont mis en cache jusqu'à la prochaine transaction validée (2 min au plus).
- Listes paginées en deux temps : identifiants triés et total (`count(*) OVER ()`), puis colonnes calculées (vidéo, enfants, disponibilité, données de l'utilisateur) pour la seule page.

### 5.2 Métadonnées
> Implémenté en M3 : `internal/metadata` (contrat), `nfo` (local), `tmdb`, `musicbrainz`, `httpx` (client limité en débit + cache disque), `refresh` (orchestration), `internal/images` (cache adressé par contenu, variantes, blurhash). Statuts d'identification : `pending`, `matched`, `unmatched`, `manual`, `local`. Acceptation automatique : score ≥ 0,75 et avance ≥ 0,03 sur le 2ᵉ candidat. Sur une saison/un épisode, `providerIds.tmdb` est l'id TMDB de la saison/de l'épisode, pas de la série.

- Ordre par défaut : **NFO local / images locales** → TMDB (films, séries) → TVDB (séries, en option) → MusicBrainz + Cover Art Archive (musique) → fanart.tv (logos et fonds, en option).
- Identification : ids présents dans le nom ou le NFO ; sinon recherche par titre + année, avec un score de confiance. En dessous du seuil, l'item est marqué **« à identifier »** (correction manuelle dans l'UI).
- Fusion champ par champ, en respectant les **champs verrouillés**. Rafraîchissement périodique pour les séries en cours.
- Clés API fournies par l'utilisateur, ou une clé projet par défaut pour TMDB. Cache HTTP et rate limiting par provider.

### 5.3 Lecture
1. Le client envoie son **profil de capacités**, détecté côté navigateur via `MediaSource.isTypeSupported` / `canPlayType` : conteneurs, codecs vidéo/audio, HDR, débit max.
2. `stream/decision` choisit :
   - **Direct Play** : fichier servi tel quel avec HTTP Range (MP4/WebM/MKV-compatible + codecs supportés).
   - **Remux** (Direct Stream) : codecs OK mais conteneur KO → `ffmpeg -c copy` vers HLS fMP4.
   - **Transcode** : vidéo et/ou audio réencodés (H.264/AAC par défaut, HEVC/AV1 si supportés), HLS fMP4 avec segments générés à la demande et seek → relance de FFmpeg à la bonne position.
3. Sous-titres : texte (SRT/ASS/VTT) → WebVTT servi à part ; image (PGS/VobSub) → incrustation (burn-in) seulement si l'utilisateur les active.
4. Accélération matérielle (VAAPI, QSV, NVENC, VideoToolbox, AMF) en option, avec détection au démarrage.
5. Le client remonte périodiquement sa progression (`POST /api/v1/playback/progress`), et la session FFmpeg est tuée après inactivité.

---

### 5.4 Sources distantes (M5b — conception)
```
                 ┌────────────── serveur Aether principal ──────────────┐
 navigateur ───► │ API · UI · base · métadonnées · décision de lecture  │
                 │        VFS : local │ agent │ sftp                    │
                 └───────────┬──────────────┬───────────────┬──────────┘
                     dossier local     HTTPS + jeton      SFTP (clé)
                                    ┌──────────────┐   ┌──────────┐
                                    │ Agent Aether │   │ seedbox  │
                                    │ fichiers,    │   └──────────┘
                                    │ ffprobe,     │
                                    │ FFmpeg (HLS) │
                                    └──────────────┘
```
- Une bibliothèque = plusieurs **sources** ; l'identité des fichiers devient `source_id + chemin relatif`.
- **VFS** : `Walk`, `Stat`, `Open` (lecture par plages), `InputURL` pour FFmpeg. Les sources non locales sont servies à FFmpeg par un proxy HTTP interne (Range) ; l'agent peut aussi analyser et transcoder lui-même pour éviter de faire transiter le fichier complet.
- **Agent** : même binaire en mode `aether agent`, sans base de données ni UI ; expose listing récursif (taille, mtime), lecture par plages, probe, sessions HLS ; pousse les changements de fichiers au serveur.
- Source hors ligne : rien n'est supprimé (même règle que les racines indisponibles), les éléments sont marqués indisponibles.

### 5.5 Synchronisation externe (M6b)
> Implémenté en M6b : `internal/syncsvc` (contrat `docs/API.md` § Synchronisation).

- **Fournisseurs** derrière une interface commune `Provider` (`trakt.go`, `simkl.go`) : appairage (Trakt *device code*, Simkl *PIN*), rafraîchissement / révocation des jetons, activité distante, lecture et écriture par fonction, scrobble. URL de base configurable (`AETHER_TRAKT_API_URL`, `AETHER_SIMKL_API_URL`) : les tests utilisent des faux serveurs `httptest`, jamais le réseau.
- **Identifiants d'application** : configuration (TOML / env) prioritaire, sinon saisis par un admin et chiffrés (`sync_apps`, `internal/secret`). Jetons utilisateur chiffrés dans `sync_connections` ; rafraîchis une heure avant expiration ou après un 401 (verrou par connexion), refus → `reauth_required`.
- **Scrobbling** : les événements du lecteur (`progress.go` : début, pause/reprise, fin selon `watchedEnough`, arrêt anticipé → pause) deviennent des tâches `sync.scrobble` (1 worker, ordre FIFO, événements périmés ignorés).
- **Synchronisation** (`sync.run`, clé par connexion, donc jamais deux fois en parallèle) : import complet à la connexion, puis toutes les 30 min, ~1 min après un changement local (débounce via `EnqueueAfter`) et à la demande. Fusion **à trois points** par fonction (`merge.go`) : état local, état distant, dernier état synchronisé (`sync_state`) ; changement d'un seul côté recopié, des deux côtés → la date la plus récente gagne et un conflit est journalisé (`sync_events`). Si l'activité distante (`last_activities`) n'a pas bougé, l'état distant est celui de `sync_state` (aucun téléchargement) ; récupération complète au moins toutes les 24 h.
- **Correspondance** par `providerIds` (films : tmdb → imdb → tvdb ; séries : tvdb → tmdb → imdb ; épisodes : série + saison + numéro, fichiers multi-épisodes compris), limitée aux éléments visibles par l'utilisateur.
- **Débit** : client HTTP propre (POST/DELETE authentifiés) avec budgets lecture/écriture (`httpx.Limiter`), pause sur `X-Ratelimit` épuisé ou 429 → `jobs.RetryAfter` (prochaine tentative après `Retry-After`, 3 essais maximum, jamais de boucle).
- **Watchlist locale** `user_watchlist` (retraits conservés pour dater les conflits) ; `user_item_data` gagne `played_changed_at` et `rated_at`.

## 6. API (aperçu)
```
POST /api/v1/auth/login | /logout          GET /api/v1/me
GET  /api/v1/libraries                      POST /api/v1/libraries/{id}/scan
GET  /api/v1/items?library=&kind=&genre=&year=&sort=&cursor=
GET  /api/v1/items/{id}  /children  /similar
GET  /api/v1/home   (reprendre, à suivre, récemment ajoutés, collections)
GET  /api/v1/search?q=
GET  /api/v1/images/{itemId}/{type}?w=&format=
POST /api/v1/playback/{itemId}/start  → décision + URL (direct ou master.m3u8)
POST /api/v1/playback/progress | /stop
PUT  /api/v1/items/{id}/userdata  (vu, favori, note)
CRUD /api/v1/collections  /api/v1/users (admin)
GET  /api/v1/events  (SSE)
```

## 7. Sécurité et vie privée
- Aucun compte cloud. Aucune télémétrie par défaut.
- Mots de passe argon2id, tokens opaques hachés en DB, rate limit sur le login.
- Bibliothèques autorisées par utilisateur, contrôle parental par classification.
- Chemins servis **uniquement** via les ids d'items : jamais de chemin brut accepté côté API (pas de path traversal).
- Les arguments FFmpeg sont construits en liste, jamais via un shell.
- **Requêtes sortantes vers des URL non fiables** (images des NFO et des fournisseurs, redirections) : client `internal/netguard`. Seuls http et https sont permis ; l'adresse est vérifiée **à la connexion** (`net.Dialer.Control`, après résolution DNS, donc résistant au DNS rebinding) et refusée si elle n'est pas publique (boucle locale, réseaux privés, lien local dont 169.254.169.254, ULA `fc00::/7` dont `fd00:ec2::254`, multicast, CGNAT, plages réservées, adresses IPv4 encapsulées en IPv6). Avec un proxy sortant configuré, la cible est vérifiée avant de lui être confiée. Délais (connexion, en-têtes, total), 5 redirections au plus, taille maximale lue par l'appelant (25 Mio pour les images).

## 8. Décisions ouvertes
- [x] **Go vs Rust** → Go (validé)
- [x] **SvelteKit vs React** → SvelteKit (validé)
- [ ] SSE vs WebSocket pour les événements (recommandation : SSE en v1, WebSocket si SyncPlay)
- [ ] Postgres optionnel dès la v1 ? (recommandation : non)
