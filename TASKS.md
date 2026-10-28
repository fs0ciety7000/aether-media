# Aether — Tâches

Légende : `[ ]` à faire · `[~]` en cours · `[x]` fait · 🔒 bloqué par une validation

## M-1 — Cadrage ✅
- [x] T-001 Étudier l'architecture de Jellyfin (scan, naming, providers, encodage, API)
- [x] T-002 Rédiger ARCHITECTURE.md (draft)
- [x] T-003 Rédiger DESIGN_SYSTEM.md (draft)
- [x] T-004 Rédiger PLAN.md et TASKS.md
- [x] T-005 Validation de la stack : Go + SQLite + SvelteKit
- [x] T-006 Validation de la direction artistique (implicite au lancement de M1, revue visuelle à confirmer)

## M0 — Fondations ✅
- [x] T-010 Init du module Go, arborescence `cmd/` `internal/`, Makefile / Taskfile
- [x] T-011 Config TOML + env, chemins data/cache/transcode
- [x] T-012 SQLite (WAL) + goose + sqlc, première migration (libraries, items, users)
- [x] T-013 Serveur HTTP, logs structurés (slog), healthcheck, arrêt propre
- [x] T-014 Init SvelteKit (adapter-static), embarqué via `embed.FS`
- [x] T-015 CI GitHub Actions : lint, tests, build multi-OS (pas encore exécutée sur GitHub)
- [x] T-016 Dockerfile (avec FFmpeg) + docker-compose d'exemple
- [ ] T-017 Alléger l'image Docker (787 Mo avec le FFmpeg Debian) → FFmpeg statique ou jellyfin-ffmpeg

## M1 — Design system & shell ✅
- [x] T-020 `tokens.css` + 5 thèmes + sélecteur de thème
- [x] T-021 Polices auto-hébergées (remplacées par Nunito en M1b)
- [x] T-022 Composants : Button, Card (poster/thumb), Row, Hero, Input, Chip, Skeleton, Toast
- [x] T-023 App shell (sidebar / barre basse, header au scroll)
- [x] T-024 Page `/design` (galerie de composants) + test de contraste en CI
- [x] T-025 Images réelles dans le Hero : `object-position` mobile, logos de titre
- [x] T-026 Flèches des rangées accessibles au clavier

## M1b — Refonte UI (référence Nexify) ✅
- [x] T-027 Thème « Nuit » par défaut (bleu nuit + teal adouci), 5 thèmes alternatifs adaptés
- [x] T-028 Nunito partout (Cormorant Garamond et Instrument Sans retirés)
- [x] T-029 Shell dashboard : sidebar en tuiles, top bar (recherche, notifications, utilisateur), tab bar mobile
- [x] T-02A Accueil : hero en carte, panneau Catégories, mosaïque « Reprendre », rangées
- [ ] T-02B Validation visuelle M1b par le PO

## M2 — Scan & indexation ✅
- [x] T-030 `naming` : films (titre, année, edition, ids *arr)
- [x] T-031 `naming` : séries (SxxEyy, multi-épisodes, par date, specials)
- [x] T-032 `naming` : musique (artiste / album / piste, disques)
- [x] T-033 Corpus de tests de noms (≥ 300 cas → ~600 cas + fuzzing)
- [x] T-034 Walker + règles d'ignore + fingerprint incrémental
- [x] T-035 Resolvers movie / series / season / episode / album / track
- [x] T-036 ffprobe → MediaInfo normalisé (streams, chapitres)
- [x] T-037 Job runner persistant + SSE de progression
- [x] T-038 Watcher fsnotify + scan périodique de secours
- [ ] T-039 Extras (bandes-annonces, making-of) rattachés à leur film/série
- [ ] T-03A Bibliothèques « mixed » : structure séries mal reconnue si pas à la racine

## M3 — Métadonnées ✅
- [x] T-040 Interface Provider + orchestrateur (ordre, fusion, verrous)
- [x] T-041 Provider NFO (Kodi) + images locales
- [x] T-042 Provider TMDB (films, séries, casting, collections)
- [x] T-043 Provider MusicBrainz + Cover Art Archive
- [x] T-044 Cache d'images, redimensionnement, blurhash, couleur dominante
- [x] T-045 Flux « à identifier » + correction manuelle (API ; écran en M4)
- [x] T-046 Validation TMDB en conditions réelles (films, série, saison, épisodes, casting, images, recherche manuelle)
- [ ] T-04C Recherche manuelle sans année : le titre exact plus ancien peut être classé sous un homonyme récent (préremplir l'année dans l'UI)
- [ ] T-047 Images des personnes (photos acteurs) servies via le cache, pas en URL distante
- [ ] T-048 Rafraîchissement périodique des séries en cours (nouveaux épisodes)
- [ ] T-049 Garde SSRF du téléchargeur d'images (bloquer IP privées/loopback pour les URL issues des NFO)
- [ ] T-04A fanart.tv (logos, fonds d'artistes) — optionnel
- [x] T-04B API système : indiquer si TMDB est actif (pour l'UI « à identifier »)

## M4 — Parcourir & rechercher ✅
- [x] T-050 API items (filtres, tri, curseur), home, détail
- [x] T-051 FTS5 + endpoint de recherche
- [x] T-052 Pages Accueil, Bibliothèque, Détail
- [x] T-053 Page Recherche (instantanée, résultats groupés)
- [x] T-054 Écran « À identifier » (recherche de candidats, choix), édition d'un élément, gestion des bibliothèques (ajout, scan en direct, suppression)
- [x] T-055 Script de bibliothèque de démonstration (`web/scripts/demo-library.sh`)
- [ ] T-056 Défilement infini testé sur une grosse bibliothèque (> 60 éléments)
- [ ] T-057 Page personne (filmographie) au lieu de `/parcourir?personne=`

## M5 — Lecture ✅
- [x] T-060 Détection des capacités du navigateur
- [x] T-061 Moteur de décision direct / remux / transcode
- [x] T-062 Direct play (HTTP Range)
- [x] T-063 HLS fMP4 à la demande + seek + nettoyage des sessions
- [x] T-064 Sous-titres (→ VTT, burn-in PGS)
- [x] T-065 Lecteur web stylé (pistes, plein écran, raccourcis clavier)
- [x] T-066 Accélération matérielle (détection + options ; paramètres testés, pas de GPU dans l'environnement de dev)
- [x] T-067 Matrice de tests de lecture (Playwright)
- [x] T-03B Sous-titres externes
- [ ] T-068 Cache des images clés en base (remux MKV/TS : lecture complète du fichier au 1er lancement)
- [ ] T-069 Chapitres dans l'API et sur la barre de progression
- [ ] T-06A Valider l'accélération matérielle sur vrai GPU (VAAPI/NVENC/QSV)
- [ ] T-06B Mini-lecteur audio persistant

## M5b — Sources distantes ✅
- [x] T-0B0 Migration `library_paths` → `library_sources` (type local | agent | sftp, config, état)
- [x] T-0B1 Abstraction VFS (Walk/Stat/Open avec Range, URL d'entrée FFmpeg) ; scanner, probe, lecture directe branchés dessus
- [x] T-0B2 Proxy HTTP interne (Range) pour exposer les sources non locales à FFmpeg/ffprobe
- [x] T-0B3 Agent Aether : `aether agent`, appairage par jeton, API fichiers, probe sur place
- [ ] T-0B4 Agent : transcodage/remux près des fichiers, relayé par le serveur principal
- [x] T-0B5 Agent : notification des changements (watcher local → serveur)
- [x] T-0B6 Source SFTP (clé / mot de passe, reconnexion, lecture par plages)
- [x] T-0B7 UI : ajout de sources par type, test de connexion, état en ligne/hors ligne
- [x] T-0B8 Tests de bout en bout : bibliothèque mixant local + agent + SFTP
- [ ] T-0B9 Codes d'erreur stables pour les tests de connexion (aujourd'hui texte libre traduit côté UI)
- [ ] T-0BA SFTP : parcours parallèle des gros arbres, détection des changements (polling léger)
- [ ] T-0BB Agent : HTTPS par défaut (certificat auto-signé + empreinte épinglée)
- [ ] T-0BC Proxy interne : supporter un serveur lié à une IP non-loopback

## M5c — Appairage simplifié des agents
- [x] T-0E0 Agent sans configuration : état persistant, partages gérés à chaud
- [x] T-0E1 Code d'appairage (rotation, usage unique, limitation), page d'état locale
- [x] T-0E2 Découverte UDP sur le réseau local
- [x] T-0E3 Serveur : agents appairés, appairage, navigation, création de partages, désappairage
- [x] T-0E4 Sélecteur de dossiers pour les sources locales du serveur
- [x] T-0E5 Fichiers cachés jamais exposés par l'agent (ex. ~/.ssh si un dossier personnel est partagé)
- [x] T-0E6 UI : parcours guidé (agents connus, découverte, code, choix des dossiers)
- [ ] T-0E7 Installation de l'agent en service (systemd, launchd, service Windows)

## M6 — Utilisateurs & progression ✅
- [x] T-070 Comptes, argon2id, sessions, rôles (premier compte = admin, limitation des connexions, contrôle d'Origin)
- [x] T-071 Accès par bibliothèque + contrôle parental (âge hérité, non classés masquables)
- [x] T-072 Progression, vu, favoris
- [x] T-073 Reprendre / À suivre
- [x] T-074 Collections (manuelles + TMDB)
- [x] T-03C Chemins disque et endpoints d'admin réservés aux administrateurs
- [ ] T-075 Notes personnelles affichées dans l'UI (l'API les accepte déjà)
- [ ] T-076 Avatars d'utilisateurs (image) — aujourd'hui initiales colorées

## M6b — Synchronisation Trakt / Simkl
- [ ] T-0C0 Connexions par utilisateur (Trakt device code, Simkl PIN), stockage chiffré des jetons, rafraîchissement
- [ ] T-0C1 Scrobbling en direct (start/pause/stop depuis les pings du lecteur)
- [ ] T-0C2 Historique vu : import initial + synchronisation incrémentale bidirectionnelle
- [ ] T-0C3 Progression de reprise (Trakt playback) bidirectionnelle
- [ ] T-0C4 Watchlist et notes
- [ ] T-0C5 UI : réglages par utilisateur, état de synchronisation, journal des conflits

## M7 — Finition & distribution
- [ ] T-080 Assistant de premier lancement
- [ ] T-081 Admin : bibliothèques, utilisateurs, tâches, journaux
- [ ] T-082 PWA (manifest, service worker)
- [ ] T-083 Paquets : Docker multi-arch, .deb/.rpm, Homebrew, Windows
- [ ] T-084 Docs utilisateur + audit a11y / perfs → release v0.1.0

## M8 — Fédération (v1.1)
- [ ] T-0D0 Appairage serveur ↔ serveur, bibliothèques partagées, droits par utilisateur
- [ ] T-0D1 Lecture relayée ou directe vers le serveur propriétaire
