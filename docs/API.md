# Aether — API HTTP v1

Base : `/api/v1`. JSON UTF-8, camelCase. Erreurs : `{"error": {"code": "...", "message": "..."}}` avec le statut HTTP adéquat.
Pas encore d'authentification (M6) : ne pas exposer le serveur sur un réseau non fiable.

## Types communs

```ts
type Image = { url: string; width?: number; height?: number; blurhash?: string; dominant?: string }
// url est versionnée (cache immuable). Ajouter "&w=<px>" pour une variante redimensionnée
// (largeurs normalisées : 92, 154, 185, 342, 500, 780, 1280, 1920, 3840).

type Kind = "movie" | "series" | "season" | "episode" | "artist" | "album" | "track"

type Item = {
  id: number; libraryId: number; parentId?: number; kind: Kind
  title: string; originalTitle?: string; year?: number; overview?: string; tagline?: string
  runtimeMs?: number; indexNumber?: number; indexNumberEnd?: number; parentIndexNumber?: number
  edition?: string; airDate?: string; premiereDate?: string; endDate?: string
  status?: "continuing" | "ended"; officialRating?: string; communityRating?: number; collection?: string
  providerIds: Record<string, string>; path?: string; dateAdded: string
  identifyStatus: "pending" | "matched" | "unmatched" | "manual" | "local"
  lockedFields: string[]
  images: Partial<Record<"poster" | "backdrop" | "logo" | "thumb" | "banner", Image[]>>
  childCount?: number          // saisons d'une série, épisodes d'une saison, albums, pistes
  video?: { resolution: "4K" | "1080p" | "720p" | "SD"; hdr?: string } // films, épisodes
}

type ItemDetail = Item & {
  childCount: number
  genres: string[]; studios: string[]
  people: { id: number; name: string; kind: string; role?: string; image?: string }[]
  ancestors: { id: number; kind: Kind; title: string }[]   // racine d'abord (série, saison…)
  mediaFiles: {
    id: number; path: string; size: number; part?: number; container?: string
    durationMs?: number; bitrate?: number; probed: boolean; probeError?: string
    streams: { index: number; type: string; codec: string; profile?: string; width?: number; height?: number
               frameRate?: number; bitDepth?: number; hdr?: string; channels?: number; channelLayout?: string
               language?: string; title?: string; default: boolean; forced: boolean; imageSub?: boolean }[]
  }[]
}

type Library = { id: number; name: string; kind: "movies" | "shows" | "music" | "mixed"
                 paths: string[]; counts: Record<string, number>; createdAt: string }
```

## Système
- `GET /system/info` → `{ name, version, commit, startedAt, setupRequired, metadata: { tmdb: boolean, musicbrainz: boolean } }`

## Bibliothèques
- `GET /libraries` → `Library[]`
- `POST /libraries` `{ name, kind, paths: string[] }` → `201 Library` (scan lancé automatiquement). Erreurs : 400 (validation), 409 (dossier déjà couvert).
- `GET /libraries/{id}` → `Library`
- `DELETE /libraries/{id}` → 204
- `POST /libraries/{id}/scan[?full=true]` → `202 { queued }`
- `GET /libraries/{id}/filters` → `{ genres: {name, count}[], years: {min, max} | null, resolutions: string[], hasHdr: boolean }`

## Éléments
- `GET /items` → `{ items: Item[], nextCursor?: string, total: number }`
  - `library`, `parent`, `kind` (liste séparée par virgules), `genre`, `person` (id), `yearFrom`, `yearTo`,
    `resolution` (`4K|1080p|720p|SD`), `hdr=true`, `identify` (statut),
  - `sort` : `title` (défaut) | `dateAdded` | `year` | `rating` | `runtime` | `premiereDate` | `random`
  - `order` : `asc` | `desc` (défaut : asc pour title, desc sinon) · `limit` 1–200 (défaut 50) · `cursor` opaque
  - Sans `parent`, seuls les éléments de premier niveau sont listés si `kind` est omis (films, séries, artistes).
- `GET /items/{id}` → `ItemDetail`
- `GET /items/{id}/similar?limit=12` → `Item[]` (genres communs, même type)
- `PATCH /items/{id}` `{ title?, originalTitle?, overview?, tagline?, year?, officialRating?, unlock?: string[] }` → `ItemDetail` (les champs modifiés sont verrouillés)
- `POST /items/{id}/refresh[?research=true]` → `202 { queued }`
- `POST /items/{id}/identify/search` `{ title?, year? }` → `Candidate[]` · 503 si aucun fournisseur actif
  - `Candidate = { provider, providerIds, title, year?, overview?, imageUrl?, score }`
- `POST /items/{id}/identify` `{ providerIds }` → 202
- `GET /items/{id}/images/{type}?idx=0&w=342` → image (Cache-Control immuable, ETag)

## Accueil
- `GET /home` →
```ts
{
  hero: Item[]                       // ≤ 5 éléments récents avec fond (backdrop)
  continueWatching: Item[]           // vide jusqu'à M6
  recentlyAdded: { library: { id: number; name: string; kind: string }; items: Item[] }[]
  categories: { genre: string; count: number; image?: Image }[]   // ≤ 8 genres les plus fournis
  recommended: Item[]                // sélection (notes / aléatoire), ≤ 16
}
```

## Recherche
- `GET /search?q=<texte>&limit=8` (q ≥ 2 caractères ; préfixes, insensible aux accents) →
```ts
{ query: string
  results: { movie: Item[]; series: Item[]; episode: Item[]; artist: Item[]; album: Item[]; track: Item[]
             person: { id: number; name: string; image?: string; count: number }[] } }
```

## Tâches & événements
- `GET /jobs` → `{ [kind]: { [status]: count } }`
- `GET /events` (SSE), chaque message : `{ type, time, data }`
  - `library.scan.started` `{ libraryId, name }` · `library.scan.progress` `{ libraryId, processed, total }`
  - `library.scan.finished` `{ libraryId, files, added, updated, unchanged, removed, skipped, unavailable?, durationMs }`
  - `item.updated` `{ id, kind }` · `item.image` `{ id, type }`
  - **Destinataires** : `library.scan.started`, `library.scan.progress`, `source.status` et `agent.status` ne sont envoyés qu'aux administrateurs ; `library.scan.finished` à tous, sans le champ `unavailable` (chemins, consultables dans les journaux) ; `sync.status` et `userdata.changed` uniquement à l'utilisateur concerné.

## Précisions
- `genre` : une seule valeur (nom exact, insensible à la casse).
- `Candidate.score` : 0 à 1.
- `Person.image` : URL distante (TMDB) pour l'instant ; passera par le cache d'images (T-047).
- `runtimeMs` : durée réelle du fichier (ffprobe) quand elle est connue, sinon celle des métadonnées.

## Lecture (M5)

```ts
// Capacités détectées par le navigateur (MediaSource.isTypeSupported / canPlayType).
type ClientProfile = {
  containers: string[]        // lecture directe possible : "mp4", "webm", "mov", "mkv"(rare), "ogg", "mp3", "flac", "m4a", "wav"
  videoCodecs: string[]       // "h264", "hevc", "av1", "vp9", "vp8"
  audioCodecs: string[]       // "aac", "mp3", "opus", "vorbis", "flac", "ac3", "eac3", "alac", "pcm"
  hdr: boolean                // affichage HDR (matchMedia("(dynamic-range: high)") + codec 10 bits)
  maxHeight?: number          // ex. 2160, 1080 (écran / choix utilisateur)
  maxBitrate?: number         // bits/s, choix utilisateur (« Qualité »), 0 = illimité
  hls: "native" | "mse"       // Safari natif, sinon hls.js (MSE)
}

type Track = { index: number; language?: string; title?: string; codec: string; channels?: number
               default: boolean; forced?: boolean; external?: boolean }

type PlaybackInfo = {
  sessionId: string
  method: "direct" | "remux" | "transcode"
  reasons: string[]           // pourquoi pas « direct » (ex. "container", "videoCodec", "audioCodec", "bitrate", "hdr", "subtitleBurn")
  url: string                 // direct : /api/v1/media/{fileId}/stream ; sinon : master.m3u8 de la session
  mediaFileId: number
  durationMs: number
  startMs: number             // position réelle de départ (alignée sur un segment en HLS)
  audioTracks: Track[]
  subtitleTracks: (Track & { delivery: "vtt" | "burn"; url?: string })[]   // url pour delivery "vtt"
  selectedAudio: number       // index de piste (stream index ffprobe)
  selectedSubtitle: number    // -1 = aucun
  video?: { codec: string; width: number; height: number; hdr?: string; copied: boolean }
}
```

- `POST /playback/{itemId}` `{ profile: ClientProfile, mediaFileId?, audioIndex?, subtitleIndex?, startMs? }` → `PlaybackInfo`
  - Changer de piste audio/sous-titre (burn) ou de qualité = nouvel appel (l'ancienne session est arrêtée avec `DELETE`).
  - 503 `transcode_busy` si le nombre maximal de transcodages simultanés est atteint.
- `POST /playback/sessions/{sid}/ping` `{ positionMs, paused }` → 204 (toutes les 10 s ; une session sans ping depuis 60 s est arrêtée)
- `DELETE /playback/sessions/{sid}` → 204
- HLS : `GET /playback/sessions/{sid}/master.m3u8`, `.../main.m3u8`, `.../init.mp4`, `.../{n}.m4s` (segments générés à la demande ; un segment lointain relance FFmpeg à cette position)
- `GET /media/{fileId}/stream` : fichier brut, HTTP Range (lecture directe)
- `GET /media/{fileId}/subtitles/{index}.vtt` : sous-titre texte converti en WebVTT (interne ou externe)

Précisions :
- `subtitleIndex` sur une piste **image** (PGS, VobSub) déclenche l'incrustation (transcodage). Sur une piste **texte**, il ne déclenche rien côté serveur : la piste est fournie en WebVTT et activée par le lecteur.
- La playlist HLS couvre **tout le fichier** (horodatage absolu) ; `startMs` indique où commencer la lecture.
- `video.codec` décrit la source ; `copied: false` signifie qu'elle est réencodée en H.264.
- Une même liste de codecs sert à la lecture directe et au remux : le lecteur n'y déclare que les codecs lisibles dans les deux cas (fMP4/HLS).

## Sources (M5b)

Une bibliothèque regroupe une ou plusieurs **sources** : dossier local, Agent Aether (machine distante que l'on contrôle, voir `docs/AGENT_PROTOCOL.md`) ou serveur SFTP (seedbox).

```ts
type Source = {
  id: number; kind: "local" | "agent" | "sftp"
  name: string                 // libellé affiché (défaut : hôte + dossier)
  root: string                 // local : chemin absolu · agent : sous-dossier du partage ("" = racine) · sftp : chemin distant absolu
  status: "online" | "offline" | "unknown"; lastError?: string; lastSeen?: string
  agent?: { url: string; share: string; hostname?: string; version?: string }
  sftp?: { host: string; port: number; user: string; auth: "password" | "key"; hostKey: string }  // empreinte SHA256:...
}
// Les secrets (jeton, mot de passe, clé privée) ne sont jamais renvoyés.

type SourceInput = {
  kind: "local" | "agent" | "sftp"; name?: string; root: string
  agent?: { url: string; token: string; share: string }
  sftp?: { host: string; port?: number; user: string; password?: string; privateKey?: string; passphrase?: string; hostKey?: string }
}
```

- `Library` gagne `sources: Source[]` (le champ `paths` reste présent : chemins des sources locales).
- `POST /libraries` `{ name, kind, sources: SourceInput[] }` (`paths: string[]` reste accepté comme raccourci pour des sources locales).
- `POST /sources/test` `SourceInput` → `{ ok: boolean, error?: string, hostKey?: string, agent?: { hostname, version, shares: string[] } }`
  - SFTP : sans `hostKey`, la connexion renvoie l'empreinte de l'hôte (`ok: false`, `error: "host_key_unknown"`, `hostKey`) que l'utilisateur confirme ; elle est ensuite exigée (protection contre l'usurpation).
  - Agent avec `share: ""` : connexion seule, `{ ok: true, agent: { …, shares } }` pour proposer la liste des partages.
- `POST /libraries/{id}/sources/{sid}/test` → `{ ok, error? }` : teste une source enregistrée avec ses identifiants stockés (met aussi son état à jour).
- `POST /libraries/{id}/sources` `SourceInput` → `201 Source` (scan lancé)
- `PATCH /libraries/{id}/sources/{sid}` `Partial<SourceInput>` → `Source` (ex. nouveau jeton / mot de passe ; les objets `agent`/`sftp` peuvent être partiels, secrets absents = inchangés). La connexion n'est re-testée que si la cible change (dossier, URL, hôte, identifiants) : renommer une source hors ligne reste possible.
- `DELETE /libraries/{id}/sources/{sid}` → 204 (les éléments de cette source disparaissent au prochain scan)
- `Item.available` : `false` quand tous les fichiers de l'élément sont sur des sources hors ligne (affichage grisé, lecture désactivée).
- SSE : `source.status` `{ id, libraryId, status, lastError? }`.

## Comptes, progression, collections (M6)

### Authentification
- Session par cookie `aether_session` (HttpOnly, SameSite=Lax, Secure en HTTPS) **ou** en-tête `Authorization: Bearer <jeton>` (clients non navigateur). Toute la route `/api/v1` exige une session, **sauf** : `GET /system/info`, `POST /auth/login`, `POST /setup`.
- Requêtes modifiantes avec cookie : l'en-tête `Origin` (s'il est présent) doit correspondre à l'hôte (protection CSRF).
- 401 `unauthenticated` (pas de session) · 403 `forbidden` (droits insuffisants).
- `GET /system/info` gagne `setupRequired: boolean` (aucun compte encore).

```ts
type User = { id: number; name: string; isAdmin: boolean
              allLibraries: boolean; libraryIds: number[]   // accès si !allLibraries
              maxRatingAge?: number                         // contrôle parental : 0, 6, 10, 12, 16, 18 ; absent = aucun
              blockUnrated: boolean
              prefs: { audioLanguage?: string; subtitleLanguage?: string; subtitleMode?: "auto" | "always" | "forced" | "off" }
              createdAt: string; lastSeen?: string }
type UserData = { positionMs: number; played: boolean; playCount: number; lastPlayedAt?: string
                  favorite: boolean; rating?: number /* 0–10 */ }
```

- `POST /setup` `{ name, password }` → `201 { user, token }` + cookie — uniquement si aucun compte (sinon 409) ; crée l'administrateur.
- `POST /auth/login` `{ name, password, device? }` → `{ user, token }` + cookie · 401 `invalid_credentials` · 429 après 10 échecs / 15 min / IP.
- `POST /auth/logout` → 204 (session courante révoquée).
- `GET /me` → `User` · `PATCH /me` `{ prefs?, currentPassword?, newPassword? }` → `User` (mot de passe ≥ 8 caractères ; changer le mot de passe révoque les autres sessions).
- `GET /me/sessions` → `{ id, device, client, current, lastSeen, createdAt }[]` (`id` est une **chaîne** : entier 64 bits) · `DELETE /me/sessions/{id}` → 204.
- Administration (admin) : `GET /users` → `User[]` (dans `PATCH`, `maxRatingAge: null` retire la limite) · `POST /users` `{ name, password, isAdmin?, allLibraries?, libraryIds?, maxRatingAge?, blockUnrated? }` → `201 User` · `PATCH /users/{id}` (mêmes champs, `password` pour réinitialiser) · `DELETE /users/{id}` (impossible de supprimer le dernier admin).

### Droits
- Lecture (tout utilisateur) : navigation, recherche, détail, images, lecture, ses propres données.
- Admin seulement : bibliothèques et sources, scans, tâches, utilisateurs, modification/identification/rafraîchissement des éléments, champ `path` et chemins de fichiers dans les réponses.
- Filtrage : un utilisateur ne voit que les bibliothèques autorisées ; avec `maxRatingAge`, les éléments (et les épisodes de séries) au-dessus sont masqués partout (listes, recherche, accueil, détail → 404, lecture → 404) ; `blockUnrated` masque aussi les éléments sans classification.

### Données utilisateur
- `Item.userData: UserData` sur tous les éléments (favori/note aussi sur séries, albums…) ; `Item.unplayedCount` sur les séries et saisons (épisodes non vus) ; `Item.seriesId` / `seriesTitle` sur les épisodes.
- `PUT /items/{id}/userdata` `{ played?, favorite?, rating?, positionMs? }` → `UserData`. `played: true/false` sur une série ou une saison s'applique à tous ses épisodes.
- La progression est enregistrée par la lecture : `POST /playback/{id}` associe la session à l'utilisateur ; chaque `ping` `{ positionMs, paused }` enregistre la position ; `DELETE` de session (corps optionnel `{ positionMs }` = position finale) : si position ≥ 90 % de la durée (ou reste < 2 min pour un film / < 30 s pour un épisode) → `played`, compteur +1, position remise à 0.
- `GET /items` gagne : `played=true|false`, `favorite=true`, `inProgress=true`, `sort=lastPlayed`.

### Accueil
- `continueWatching: Item[]` — films/épisodes en cours (position > 0), du plus récent au plus ancien, ≤ 12.
- `nextUp: Item[]` (nouveau) — pour chaque série commencée, le prochain épisode non vu après le dernier vu, ≤ 12 (un épisode déjà entamé apparaît dans `continueWatching`, pas ici).

### Collections
```ts
type Collection = { id: string /* "c<id>" manuelle · "t<nom>" automatique TMDB */; name: string; kind: "manual" | "tmdb"
                    count: number; images: Item["images"] /* poster/backdrop du premier élément si aucune */ ; ownerId?: number; shared: boolean }
```
- `GET /collections` → `Collection[]` (manuelles visibles par l'utilisateur + automatiques issues de `Item.collection` avec ≥ 2 éléments).
- `GET /collections/{id}/items` → `{ collection: Collection, items: Item[] }`
- `POST /collections` `{ name, shared? }` → `201 Collection` (partagée : admin seulement) · `PATCH /collections/{id}` `{ name?, shared? }` · `DELETE /collections/{id}`
- `PUT /collections/{id}/items/{itemId}` → 204 · `DELETE /collections/{id}/items/{itemId}` → 204

## Agents appairés et sélecteur de dossiers (M5c)

Ajout d'une machine sans fichier de configuration : on lance `aether agent` sur la machine, on la trouve depuis l'interface (ou on saisit son adresse), on tape le **code d'appairage** affiché par l'agent, puis on choisit les dossiers à partager. Voir `docs/AGENT_PROTOCOL.md` (v2).

```ts
type Agent = { id: number; uid: string; name: string; url: string; version?: string
               status: "online" | "offline" | "unknown"; lastError?: string; lastSeen?: string; sources: number }
type DiscoveredAgent = { uid: string; name: string; hostname: string; version: string; port: number; tls: boolean
                         addr: string; url: string; paired: boolean; agentId?: number }
type Folder = { name: string; path: string }
```
(admin)
- `GET /agents` → `Agent[]`
- `POST /agents/discover` → `DiscoveredAgent[]` (diffusion UDP ~2 s sur le réseau local)
- `POST /agents/pair` `{ url, code }` → `Agent` · 400 `invalid_source` (« code d'appairage incorrect ou expiré », « trop d'essais… ») · 502 `agent_offline` si l'agent est injoignable
- `GET /agents/{id}/browse?path=` → `{ path, parent?, dirs: Folder[] }` (sans `path` : racines de la machine)
- `GET /agents/{id}/shares` → `{ name, path, managed }[]` · `POST /agents/{id}/shares` `{ path, name? }` → `201 { name, path }` (idempotent par chemin)
- `DELETE /agents/{id}` → 204 (désappairage) · 409 `agent_in_use` si des sources l'utilisent
- `GET /fs/browse?path=` → `{ path, parent?, dirs: Folder[] }` : dossiers **du serveur** (sélecteur des sources locales)
- `SourceInput.agent` accepte `{ agentId, share }` (agent appairé, recommandé) ; `{ url, token, share }` reste possible (jeton manuel).
- `Source.agent` gagne `agentId`. SSE : `agent.status` `{ id, status, lastError? }`.

Parcours UI recommandé pour « Agent Aether » : agents connus + « Rechercher sur le réseau » + « Saisir une adresse » → (si non appairé) code → navigateur de dossiers de l'agent → `POST /agents/{id}/shares` → source `{ kind: "agent", agent: { agentId, share }, root: "" }`.

## Synchronisation Trakt / Simkl (M6b)

Chaque utilisateur relie **son** compte Trakt et/ou Simkl. L'administrateur fournit les identifiants d'application (créés sur trakt.tv/oauth/applications et simkl.com/settings/developer) par configuration (`AETHER_TRAKT_CLIENT_ID`, `AETHER_TRAKT_CLIENT_SECRET`, `AETHER_SIMKL_CLIENT_ID`, ou `[sync]` dans le TOML) **ou** par l'API d'administration ci-dessous ; la configuration est prioritaire. Les jetons (et codes d'appareil) sont chiffrés en base et **ne sont jamais renvoyés** par l'API.

```ts
type SyncService = "trakt" | "simkl"
type SyncFeature = "scrobble" | "history" | "progress" | "watchlist" | "ratings"

type SyncConnection = {
  service: SyncService
  available: boolean            // identifiants d'application configurés par un admin
  status: "disconnected"        // aucune connexion
        | "pending"             // code affiché, en attente de validation par l'utilisateur
        | "connected"
        | "reauth_required"     // jeton refusé ou expiré : reconnecter (connect → poll)
  account?: { username: string }
  supported: SyncFeature[]      // trakt : les 5 ; simkl : scrobble (/scrobble/start|pause|stop, vu à ≥ 80 %), history, watchlist, ratings
  features: Record<SyncFeature, boolean>   // options activées (défaut : toutes celles supportées)
  syncing: boolean              // une synchronisation est en file ou en cours
  connectedAt?: string; lastSyncAt?: string
  lastError?: string; lastErrorAt?: string // dernière erreur (texte destiné à l'utilisateur, sans secret)
  pending?: DeviceCode          // appairage en cours (status "pending", ou reconnexion d'un compte encore "connected"/"reauth_required")
}
type DeviceCode = {
  userCode: string              // à afficher en grand (ex. "A1B2C3D4")
  verificationUrl: string       // ex. https://trakt.tv/activate , https://simkl.com/pin
  expiresAt: string             // ISO 8601
  interval: number              // secondes minimales entre deux appels à /poll
}
type SyncEvent = {
  id: number; time: string
  kind: "conflict" | "error" | "import" | "export"
  feature: SyncFeature
  itemId?: number; itemTitle?: string
  winner?: "local" | "remote"   // conflits : côté retenu (date la plus récente)
  message: string
}
```

### Utilisateur (sa propre connexion uniquement)
- `GET /me/sync` → `SyncConnection[]` (toujours les deux services, dans l'ordre trakt, simkl ; `features` ne contient que les fonctions supportées).
- `POST /me/sync/{service}/connect` → `DeviceCode` (lance l'appairage : Trakt *device code*, Simkl *PIN*). Remplace un appairage en attente ; une connexion existante n'est remplacée qu'une fois le nouveau code validé.
  - 503 `sync_unavailable` (service non configuré par l'admin) · 502 `remote_error` (service injoignable) · 429 `rate_limited` (+ `Retry-After`).
- `POST /me/sync/{service}/poll` → `{ status: "pending" | "slow_down" | "connected" | "expired" | "denied", interval: number, connection: SyncConnection }`
  - à appeler toutes les `interval` secondes tant que `status` vaut `pending` ; `slow_down` = augmenter l'intervalle (la nouvelle valeur est renvoyée) ; un appel trop rapproché renvoie `pending` sans interroger le service.
  - `connected` : l'import initial est lancé en tâche de fond (`syncing: true`). `expired` / `denied` : recommencer avec `/connect`.
  - 404 `not_pending` si aucun appairage n'est en cours.
- `PATCH /me/sync/{service}` `{ features: Partial<Record<SyncFeature, boolean>> }` → `SyncConnection` · 404 `not_connected` · 400 `unsupported_feature`.
- `POST /me/sync/{service}/run` `{ full?: boolean }` → `202 { queued: boolean }` (« synchroniser maintenant » ; `full` ignore les curseurs et compare tout) · 404 `not_connected` · 409 `reauth_required`.
- `DELETE /me/sync/{service}` → 204 : révoque le jeton chez le fournisseur (au mieux : Trakt `/oauth/revoke` ; Simkl n'a pas de révocation) et efface connexion, état et journal. Les données locales (vus, notes, watchlist) sont conservées.
- `GET /me/sync/{service}/events?limit=50` → `SyncEvent[]` (du plus récent au plus ancien, ≤ 200 ; conservés 30 jours).

### Administration
- `GET /sync/apps` → `Record<SyncService, { configured: boolean; source: "config" | "database" | "none"; clientId?: string /* masqué : "abcd…wxyz" */ }>`
- `PUT /sync/apps/{service}` `{ clientId: string, clientSecret?: string }` → même objet pour ce service (`clientSecret` requis pour Trakt) · 409 `managed_by_config` si la configuration fournit déjà ces identifiants · 400 `invalid_input`.
- `DELETE /sync/apps/{service}` → 204 (les connexions existantes passent en erreur à la prochaine utilisation).
- `GET /system/info` gagne `sync: { trakt: boolean, simkl: boolean }` (service utilisable).

### Watchlist locale
Liste « à voir » par utilisateur (films et séries), synchronisée avec la watchlist Trakt et la liste *Plan to Watch* de Simkl.
- `UserData.inWatchlist: boolean` sur tous les éléments.
- `GET /watchlist` → `Item[]` (ajouts les plus récents d'abord ; éléments visibles seulement).
- `PUT /watchlist/{itemId}` → 204 · `DELETE /watchlist/{itemId}` → 204 · 400 `invalid_kind` (ni film ni série) · 404.
- `GET /items` gagne `watchlist=true`.
- `PUT /items/{id}/userdata` : `rating: null` efface la note (synchronisé aussi).

### Comportement
- **Scrobbling** (option `scrobble`) : `POST /playback/{id}` → *start* ; ping avec `paused` qui change → *pause* / *start* ; fin de lecture (règle « vu » ci-dessus) → *stop* (Trakt et Simkl le comptent comme vu à partir de 80 %) ; `DELETE` de session avant la fin → *pause* (le service garde la progression). Simkl : mêmes événements via ses propres `/scrobble/*` ; ses points de reprise ne sont pas encore synchronisés. Asynchrone (file de tâches) : la lecture n'attend jamais le service.
- **Synchronisation** (options `history`, `progress`, `watchlist`, `ratings`) : import complet à la connexion, puis toutes les 30 minutes et ~1 minute après chaque changement local (vu/non vu, note, watchlist, progression). Comparaison à trois points (état local, état distant, dernier état synchronisé) : un changement d'un seul côté est recopié de l'autre ; s'il a changé des deux côtés, **la date la plus récente gagne** et un `SyncEvent` `conflict` est journalisé.
- Correspondance par identifiants (`providerIds` tmdb / imdb / tvdb) : films et séries directement, épisodes par la série + numéros de saison et d'épisode. Les éléments absents de la bibliothèque (ou non visibles par l'utilisateur) sont ignorés.
- Notes : Aether 0–10 (décimales) ↔ entiers 1–10 (arrondi, minimum 1).
- Progression : pourcentage côté Trakt ↔ `positionMs` (durée de l'élément) ; écart < 2 % ignoré.
- Limites de débit : requêtes espacées (Trakt : 1 écriture/s), 429 → nouvelle tentative après `Retry-After`, jamais de boucle (3 essais par tâche).
- SSE : `sync.status` `{ userId, service, status, syncing, lastError? }` après chaque synchronisation ou changement d'état ; `userdata.changed` `{ userId, itemId }` pour chaque élément modifié par un import.
