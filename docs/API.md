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
