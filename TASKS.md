# Aether — Tâches

Légende : `[ ]` à faire · `[~]` en cours · `[x]` fait · 🔒 bloqué par une validation

## M-1 — Cadrage (en cours)
- [x] T-001 Étudier l'architecture de Jellyfin (scan, naming, providers, encodage, API)
- [x] T-002 Rédiger ARCHITECTURE.md (draft)
- [x] T-003 Rédiger DESIGN_SYSTEM.md (draft)
- [x] T-004 Rédiger PLAN.md et TASKS.md
- [ ] 🔒 T-005 **Validation de la stack par le PO** (Go + SQLite + SvelteKit ?)
- [ ] 🔒 T-006 **Validation de la direction artistique** (palette, typos, thèmes)

## M0 — Fondations
- [ ] T-010 Init du module Go, arborescence `cmd/` `internal/`, Makefile / Taskfile
- [ ] T-011 Config TOML + env, chemins data/cache/transcode
- [ ] T-012 SQLite (WAL) + goose + sqlc, première migration (libraries, items, users)
- [ ] T-013 Serveur HTTP, logs structurés (slog), healthcheck, arrêt propre
- [ ] T-014 Init SvelteKit (adapter-static), embarqué via `embed.FS`
- [ ] T-015 CI GitHub Actions : lint, tests, build multi-OS
- [ ] T-016 Dockerfile (avec FFmpeg) + docker-compose d'exemple

## M1 — Design system & shell
- [ ] T-020 `tokens.css` + 5 thèmes + sélecteur de thème
- [ ] T-021 Polices auto-hébergées (Cormorant Garamond, Instrument Sans)
- [ ] T-022 Composants : Button, Card (poster/thumb), Row, Hero, Input, Chip, Skeleton, Toast
- [ ] T-023 App shell (sidebar / barre basse, header au scroll)
- [ ] T-024 Page `/design` (galerie de composants) + test de contraste en CI

## M2 — Scan & indexation
- [ ] T-030 `naming` : films (titre, année, edition, ids *arr)
- [ ] T-031 `naming` : séries (SxxEyy, multi-épisodes, par date, specials)
- [ ] T-032 `naming` : musique (artiste / album / piste, disques)
- [ ] T-033 Corpus de tests de noms (≥ 300 cas)
- [ ] T-034 Walker + règles d'ignore + fingerprint incrémental
- [ ] T-035 Resolvers movie / series / season / episode / album / track
- [ ] T-036 ffprobe → MediaInfo normalisé (streams, chapitres)
- [ ] T-037 Job runner persistant + SSE de progression
- [ ] T-038 Watcher fsnotify + scan périodique de secours

## M3 — Métadonnées
- [ ] T-040 Interface Provider + orchestrateur (ordre, fusion, verrous)
- [ ] T-041 Provider NFO (Kodi) + images locales
- [ ] T-042 Provider TMDB (films, séries, casting, collections)
- [ ] T-043 Provider MusicBrainz + Cover Art Archive
- [ ] T-044 Cache d'images, redimensionnement, blurhash, couleur dominante
- [ ] T-045 Flux « à identifier » + correction manuelle

## M4 — Parcourir & rechercher
- [ ] T-050 API items (filtres, tri, curseur), home, détail
- [ ] T-051 FTS5 + endpoint de recherche
- [ ] T-052 Pages Accueil, Bibliothèque, Détail
- [ ] T-053 Page Recherche (instantanée, résultats groupés)

## M5 — Lecture
- [ ] T-060 Détection des capacités du navigateur
- [ ] T-061 Moteur de décision direct / remux / transcode
- [ ] T-062 Direct play (HTTP Range)
- [ ] T-063 HLS fMP4 à la demande + seek + nettoyage des sessions
- [ ] T-064 Sous-titres (→ VTT, burn-in PGS)
- [ ] T-065 Lecteur web stylé (pistes, plein écran, raccourcis clavier)
- [ ] T-066 Accélération matérielle (détection + options)
- [ ] T-067 Matrice de tests de lecture (Playwright)

## M6 — Utilisateurs & progression
- [ ] T-070 Comptes, argon2id, sessions, rôles
- [ ] T-071 Accès par bibliothèque + contrôle parental
- [ ] T-072 Progression, vu, favoris
- [ ] T-073 Reprendre / À suivre
- [ ] T-074 Collections (manuelles + TMDB)

## M7 — Finition & distribution
- [ ] T-080 Assistant de premier lancement
- [ ] T-081 Admin : bibliothèques, utilisateurs, tâches, journaux
- [ ] T-082 PWA (manifest, service worker)
- [ ] T-083 Paquets : Docker multi-arch, .deb/.rpm, Homebrew, Windows
- [ ] T-084 Docs utilisateur + audit a11y / perfs → release v0.1.0
