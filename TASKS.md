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
- [x] T-017 Image Docker allégée : debian:trixie-slim + jellyfin-ffmpeg 7 (VAAPI/QSV/NVENC, zscale/tonemap, libass), étapes de compilation sur la plateforme de build (multi-arch sans émulation) — 787 → 515 Mo (compressée : 210 → 131 Mo) en amd64
- [ ] T-018 Image : mesurer et tester l'arm64 sur une vraie machine (pas d'émulation QEMU dans l'environnement de dev) ; pilotes GPU testés sur du vrai matériel (Intel, AMD, NVIDIA)

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

## M5c — Appairage simplifié des agents ✅
- [x] T-0E0 Agent sans configuration : état persistant, partages gérés à chaud
- [x] T-0E1 Code d'appairage (rotation, usage unique, limitation), page d'état locale
- [x] T-0E2 Découverte UDP sur le réseau local
- [x] T-0E3 Serveur : agents appairés, appairage, navigation, création de partages, désappairage
- [x] T-0E4 Sélecteur de dossiers pour les sources locales du serveur
- [x] T-0E5 Fichiers cachés jamais exposés par l'agent (ex. ~/.ssh si un dossier personnel est partagé)
- [x] T-0E6 UI : parcours guidé (agents connus, découverte, code, choix des dossiers)
- [x] T-0E7 `aether agent|service install|uninstall|status` : systemd (système ou `--user`), LaunchDaemon / LaunchAgent, service Windows natif (x/sys/windows/svc, journal dans un fichier), `--dry-run`
- [ ] T-0EA Service Windows : choix du compte (aujourd'hui LocalSystem) et rotation du journal ; tester install/arrêt/désinstallation sur un vrai Windows et un vrai macOS (seuls la génération et la compilation croisée sont testées)
- [ ] T-0E8 Supprimer un partage d'agent depuis l'UI (route serveur à exposer)
- [ ] T-0E9 Découverte réseau : tester sur un vrai LAN (la diffusion UDP n'est pas disponible dans l'environnement de dev)

## M6 — Utilisateurs & progression ✅
- [x] T-070 Comptes, argon2id, sessions, rôles (premier compte = admin, limitation des connexions, contrôle d'Origin)
- [x] T-071 Accès par bibliothèque + contrôle parental (âge hérité, non classés masquables)
- [x] T-072 Progression, vu, favoris
- [x] T-073 Reprendre / À suivre
- [x] T-074 Collections (manuelles + TMDB)
- [x] T-03C Chemins disque et endpoints d'admin réservés aux administrateurs
- [ ] T-075 Notes personnelles affichées dans l'UI (l'API les accepte déjà)
- [ ] T-076 Avatars d'utilisateurs (image) — aujourd'hui initiales colorées

## M6b — Synchronisation Trakt / Simkl ✅ (validation réelle à faire : T-0C6)
- [x] T-0C0 Connexions par utilisateur (Trakt device code, Simkl PIN), stockage chiffré des jetons, rafraîchissement, révocation ; identifiants d'application par config/env ou API admin (chiffrés)
- [x] T-0C1 Scrobbling en direct (start/pause/stop depuis les pings du lecteur, `/scrobble/*` chez Trakt comme chez Simkl)
- [x] T-0C2 Historique vu : import initial + synchronisation incrémentale bidirectionnelle (fusion à trois points, la date la plus récente gagne)
- [x] T-0C3 Progression de reprise (Trakt playback) bidirectionnelle
- [x] T-0C4 Watchlist (nouvelle, locale par utilisateur, API + `inWatchlist`) et notes
- [x] T-0C5 UI : réglages par utilisateur, état de synchronisation, journal des conflits, watchlist, identifiants d'application (admin)
- [ ] T-0C6 Valider contre les vrais api.trakt.tv / api.simkl.com avec de vrais identifiants — vérification faite le 2026-10-28 contre la documentation officielle (developer.trakt.tv + son openapi.json, api.simkl.org + son openapi.json) et faux serveurs alignés ; reste l'essai réel avec de vrais identifiants (aucun appel authentifié fait)
- [ ] T-0C7 Simkl : synchronisation des points de reprise (`GET /sync/playback`, `DELETE /sync/playback/{id}`) — non prise en charge aujourd'hui (Simkl ne note pas les épisodes)
- [ ] T-0CC Simkl : passer à AUTH V2 (`/oauth2/device`, jetons de 7 jours + refresh, `/oauth2/revoke`) avant le retrait d'AUTH V1 annoncé vers avril 2027 ; un client_id V2 est refusé par le flux PIN
- [ ] T-0CD Simkl : synchronisation incrémentale `date_from` (la doc menace de suspendre les client_id qui relisent toute la bibliothèque ; aujourd'hui : lecture complète quand `/sync/activities` a bougé)
- [ ] T-0C8 Budget de débit par utilisateur (aujourd'hui un seul budget par service pour tout le serveur : Trakt 1 écriture/s)
- [ ] T-0C9 Import ciblé des éléments nouvellement ajoutés à la bibliothèque (aujourd'hui : récupération complète toutes les 24 h ou « synchroniser maintenant » avec `full`)
- [ ] T-0CA Historique Trakt : visionnages multiples (plays, dates) — seuls vu / non vu et la dernière date sont synchronisés
- [x] T-0CB `stream.TestIntegrationIdleTimeout` instable : l’inactivité était comptée depuis la création de la session et non depuis son enregistrement (préparation lente = session récoltée avant le premier ping) ; corrigé en M7
- [x] T-0CE SSE filtré par destinataire (événements de synchro et de progression réservés à l'utilisateur concerné ; scans, sources et agents aux admins)
- [ ] T-0CF Publier `sync.status syncing:true` dès qu'un changement local met une synchro en file ; journal : retirer le préfixe de fonction redondant des messages
- [ ] T-0CG Membres : état hors ligne d'une source mis à jour en direct (aujourd'hui au rechargement ou à la fin d'un scan, `source.status` étant réservé aux admins)
- [ ] T-0CH Annulation explicite d'un appairage Trakt/Simkl en cours côté serveur ; action « Ma liste » dans un menu des cartes

## M7 — Finition & distribution
- [x] T-080 Assistant de premier lancement : compte admin → langue et pays des métadonnées → clé TMDB (vérifiée en direct, lien d’aide, lecture seule si définie par la configuration) → première bibliothèque (AddLibraryDialog : dossier + sélecteur, agent, SFTP) → récapitulatif ; étapes passables, reprise côté serveur (`settings.wizard`, `system/info.setupWizardPending`)
- [x] T-086 Réglages serveur persistants : `GET/PATCH /settings` (nom, langue, pays, clé TMDB chiffrée et masquée, accélération matérielle, limite de transcodages, rescan périodique), priorité configuration/env > base > défaut avec la source de chaque valeur, application à chaud sauf l’accélération (« effectif au redémarrage »), migration 00009
- [x] T-081 Admin : Réglages › Serveur (réglages, version, chemins, FFmpeg, accélération détectée), Tâches (file en direct via `jobs.changed` admin, relancer / annuler / retirer, scan complet, métadonnées, synchro), Journaux (tampon slog de 2 000 entrées, secrets masqués, filtre, recherche, suivi SSE admin, téléchargement), Lectures en cours (méthode, raisons, débit, arrêt par l’admin) ; Bibliothèques et Utilisateurs existaient déjà
- [x] T-082 PWA : manifest, icônes générées (192, 512, maskable, apple-touch-icon), service worker (app shell + assets versionnés, jamais /api ni flux), écran et page hors ligne, invitation discrète à installer, métadonnées iOS et zones sûres
- [ ] T-087 Valider l’ajout à l’écran d’accueil sur un vrai iPhone / iPad (vérifié en émulation iPhone sous Chromium : métadonnées Apple, manifest, zones sûres ; pas d’appareil iOS ni de WebKit dans l’environnement de dev)
- [ ] T-088 Accélération matérielle appliquée à chaud (aujourd’hui au redémarrage) et test sur vrai GPU (voir T-06A)
- [ ] T-089 Contention SQLite (`database is locked`) pendant un rafraîchissement complet des métadonnées en parallèle d’un scan : les tâches sont réessayées et finissent, mais à étudier (pool d’écriture unique, `busy_timeout` plus long)
- [ ] T-08A Garde SSRF des images d’NFO (T-049) : une URL `http://127.0.0.1/…` est encore suivie (vu dans l’E2E M7)
- [ ] T-08B Changer la langue des métadonnées ne retraduit pas les titres déjà identifiés tant qu’on ne lance pas « Rafraîchir les métadonnées » (proposer de le lancer après l’enregistrement)
- [ ] T-08C Nom du serveur utilisé aussi pour l’appairage des agents (aujourd’hui « Aether · nom d’hôte ») et dans le titre des pages
- [x] T-083 Paquets : GoReleaser (linux amd64/arm64/armv7, darwin, windows ; tar.gz/zip ; .deb/.rpm avec compte `aether`, unités systemd, /var/lib/aether ; cask Homebrew ; images ghcr.io multi-arch via dockers_v2 ; SBOM ; checksums), workflow `release.yml` sur tag `v*`, docs/INSTALL.md
- [ ] T-085 (LICENSE AGPL-3.0 ajoutée) Première release : créer le dépôt `homebrew-tap` (puis `skip_upload: false` + secret `HOMEBREW_TAP_TOKEN`), vérifier le workflow sur un tag de test, signature/notarisation macOS et Windows
- [ ] T-084 Docs utilisateur + audit a11y / perfs → release v0.1.0

## M8 — Fédération (v1.1)
- [ ] T-0D0 Appairage serveur ↔ serveur, bibliothèques partagées, droits par utilisateur
- [ ] T-0D1 Lecture relayée ou directe vers le serveur propriétaire
