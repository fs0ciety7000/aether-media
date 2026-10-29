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
- [x] T-047 Images des personnes (photos acteurs) servies via le cache, pas en URL distante : `GET /people/{id}/image` télécharge la photo au premier appel (client protégé), garde son empreinte (`people.image_hash`, migration 00010), sert les variantes ; avatars dimensionnés et repli sur les initiales
- [ ] T-048 Rafraîchissement périodique des séries en cours (nouveaux épisodes)
- [x] T-049 Garde SSRF du téléchargeur d'images : `internal/netguard`, vérification à la connexion (`Dialer.Control`, après résolution DNS : résiste au DNS rebinding) et à chaque redirection, http/https seulement, plages non publiques refusées (boucle locale, privées, lien local et 169.254.169.254, ULA dont fd00::/8, multicast, CGNAT, réservées, IPv4 encapsulées), proxy sortant pris en compte, délais et 5 redirections au plus ; TMDB et Cover Art Archive vérifiés en réel
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
- [x] T-068 Images clés sans lecture complète : index Cues des MKV/WebM lu par quelques lectures par plages (`internal/media/mkv`, vérifié sur trois clusters échantillons), cache en base (`media_keyframes`, migration 00012), scan ffprobe réservé aux fichiers locaux (40 s au plus, puis en arrière-plan). E2E agent derrière un proxy à 50 Mbit/s et 40 ms de latence, MKV de 2,3 Go H.264 + TrueHD : démarrage du remux 180 s (scan expiré, repli sur transcodage) → 1,8 s (index : 6 lectures, 6,3 Mo), 0,8 s en cache ; premier segment 188 s → 7,5 s
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
- [x] T-0BB Agent : HTTPS par défaut (certificat auto-signé ECDSA P-256 + empreinte épinglée par le serveur, HTTPS et HTTP compatibilité sur le même port, appairage v2 lié à l'empreinte via Argon2id + HMAC, code invalidé après un échec de liaison, statut `cert_mismatch`, « Réappairer », TOFU pour le jeton manuel, FFmpeg toujours via le proxy interne) — M5d
- [x] T-0BC Proxy interne : supporter un serveur lié à une IP non-loopback (client accepté si son adresse est celle sur laquelle il a joint le serveur)
- [ ] T-0BD Appairage par PAKE (SPAKE2 / CPace) pour supprimer l'attaque hors ligne sur le code enregistré (aujourd'hui rendue coûteuse par Argon2id)
- [ ] T-0BE Agent : rechargement du certificat `tls_cert` à chaud (renouvellement Let's Encrypt sans redémarrage)
- [ ] T-0BF Avertir dans l'UI quand l'empreinte annoncée en UDP diffère de celle reçue à l'appairage

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
- [x] T-0EB `aether agent install --user --method cron` : crontab de l'utilisateur (`@reboot` + surveillance 5 min, verrou flock, journal tourné), choisie d'office sans `systemctl --user`, `--listen` / `--data-dir`, `uninstall` / `status` ; section « Seedbox sans root » dans INSTALL.md
- [ ] T-0EC Méthode cron : tester sur une vraie seedbox (Whatbox) et sur macOS / BSD (crontab et flock testés avec un faux binaire `crontab`)
- [x] T-0ED Sobriété des connexions vers un agent distant (IP du serveur bloquée par l'anti-abus d'une seedbox après un scan) : un transport par agent pour tous les appels, corps vidés avant fermeture, HTTP/2 négocié (ALPN `h2` côté agent, `ForceAttemptHTTP2` avec l'épinglage), première requête seule après inactivité, `MaxConnsPerHost` = 8 en HTTP/1.1 avec lecture par fenêtres bornées ; limite de 2 analyses / parcours simultanés par agent (`library.agent_concurrency`) ; disjoncteur par agent (hors ligne à la première erreur réseau, sonde unique après 1, 2, 5, 15, 30 min, analyses différées par `jobs.Postpone` sans consommer d'essai). Mesure E2E (30 fichiers, 40 ms de latence ajoutée) : scan 7 → 1 connexion, 90 lectures par plages 91 → 1 (agent ancien en HTTP/1.1 : 91 → 6)

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
- [x] T-089 Contention SQLite (`database is locked`) : cause racine = transactions `BEGIN DEFERRED` qui lisent puis écrivent ; si une autre connexion a validé entre-temps, la promotion en écriture échoue tout de suite (`SQLITE_BUSY_SNAPSHOT` 517, ou `SQLITE_BUSY` 5 pour éviter un interblocage), `busy_timeout` ne s’appliquant pas. Correctif : pool d’écriture d’une connexion pour `Store.Tx`, `_txlock=immediate`, `busy_timeout` 15 s ; test de stress `store.TestConcurrentWritersNoLock` (échoue avant, passe après, aussi avec `-race`)
- [x] T-08A Garde SSRF des images d’NFO (T-049) : une URL `http://127.0.0.1/…` n’est plus suivie (`images.TestIngestRefusesPrivateDestinations`)
- [ ] T-08B Changer la langue des métadonnées ne retraduit pas les titres déjà identifiés tant qu’on ne lance pas « Rafraîchir les métadonnées » (proposer de le lancer après l’enregistrement)
- [ ] T-08C Nom du serveur utilisé aussi pour l’appairage des agents (aujourd’hui « Aether · nom d’hôte ») et dans le titre des pages
- [x] T-083 Paquets : GoReleaser (linux amd64/arm64/armv7, darwin, windows ; tar.gz/zip ; .deb/.rpm avec compte `aether`, unités systemd, /var/lib/aether ; cask Homebrew ; images ghcr.io multi-arch via dockers_v2 ; SBOM ; checksums), workflow `release.yml` sur tag `v*`, docs/INSTALL.md
- [ ] T-085 (LICENSE AGPL-3.0 ajoutée) Première release : créer le dépôt `homebrew-tap` (puis `skip_upload: false` + secret `HOMEBREW_TAP_TOKEN`), vérifier le workflow sur un tag de test, signature/notarisation macOS et Windows
- [x] T-084 Docs utilisateur + audit a11y / perfs : README réécrit, `docs/GUIDE.md`, `CHANGELOG.md`, `docs/AUDIT-v0.1.0.md` (Lighthouse + axe sur 10 écrans, desktop et mobile, avant / après ; latence API sur 20 000 éléments) ; correctifs : contraste et nom accessible du menu utilisateur, compression (brotli/gzip précompressés, gzip du JSON), images LCP non différées, rangées sans reflow forcé, blurhash 16 px, `system/info.authenticated` (plus de 401 en console), parcours en deux phases, contrôle parental réécrit, facettes en cache, accueil en parallèle

## Release v0.1.0 — reste à faire avant le tag
`goreleaser check` passe ; un binaire construit comme pour le tag `v0.1.0` affiche `aether 0.1.0 (<commit>)` ; licence cohérente partout (`LICENSE` AGPL-3.0, `license: AGPL-3.0-only` des paquets, label OCI de l’image, fichier `copyright` des paquets).
- [x] T-090 CI exécutée sur GitHub pour la première fois (run n° 1 vert : lint, tests -race, build 6 cibles, image Docker, `goreleaser check`). Elle ne tournait que sur `main`, absente du dépôt : déclenchée aussi sur `ccr-*` et à la main
- [~] T-091 Répétition `v0.1.0-rc.1` : workflow Release vert, 25 fichiers publiés (archives 6 cibles, .deb/.rpm, SBOM, checksums), archive Linux vérifiée (somme, `aether version`, `serve` + `/healthz`). Corrigé : noms deb/rpm avec « ~ » renommés par GitHub, donc absents de `checksums.txt` (confirmé sur `v0.1.0-rc.2` : sommes OK pour .deb, .rpm et archives). Reste : installer .deb/.rpm sur Debian/Fedora, `docker run` de l'image ghcr.io amd64 + arm64, puis retirer la pré-release de test
- [ ] T-092 Critère de sortie M7 : installation en moins de 5 minutes sur Linux, macOS et Windows en suivant `docs/INSTALL.md` (inclut T-0EA : service Windows et LaunchAgent macOS sur de vraies machines)
- [ ] T-093 Décisions du PO pour la v0.1.0 : publication Homebrew (`homebrew-tap` + `skip_upload: false` + secret, ou report en v0.1.x comme l’annonce INSTALL.md), binaires macOS / Windows non signés (documenté : `xattr`, SmartScreen), validation visuelle T-02B
- [ ] T-094 Dater l’entrée `[0.1.0]` de `CHANGELOG.md`, coller la section dans les notes de la release en brouillon, puis tagger `v0.1.0`

## Après la v0.1.0
- Validation sur matériel et services réels : T-018 (arm64, GPU), T-06A / T-088 (accélération matérielle sur vrai GPU, puis à chaud), T-087 (PWA sur iPhone / iPad), T-0C6 (Trakt / Simkl avec de vrais comptes), T-0E9 (découverte UDP sur un vrai LAN)
- Fonctions reportées : T-039 (extras), T-03A, T-048 (nouveaux épisodes), T-04A (fanart.tv), T-04C, T-056, T-057 (page personne), T-069 (chapitres), T-06B (mini-lecteur audio), T-075, T-076, T-0B4 (transcodage par l’agent), T-0B9, T-0BA, T-0BB, T-0BC, T-0C7, T-0C8, T-0C9, T-0CA, T-0CC (Simkl AUTH V2, avant avril 2027), T-0CD, T-0CF, T-0CG, T-0CH, T-0E8, T-08B, T-08C, T-085 (signature / notarisation)
- [ ] T-095 Commande de secours `aether user reset-password` (aujourd’hui, sans second administrateur, aucun moyen hors interface)
- [ ] T-096 Variantes d’images WebP ou AVIF (Lighthouse « modern-image-formats » : pas d’encodeur WebP en Go pur, il faut passer par FFmpeg ou un encodeur dédié)
- [ ] T-097 Performance mobile (Lighthouse simulé, CPU ×4 : 55 à 93 selon les pages, accueil 55 à 62) : TBT de 130 ms à 1,6 s à l’hydratation (accueil, bibliothèque, fiche), CSS global inutilisé à ~70 % (découper `0.css` par route), rangées hors écran de l’accueil rendues plus tard, LCP mobile 2,7 à 4,3 s ; mesurer sur un vrai téléphone
- [ ] T-098 Netguard : liste d’exceptions configurable (serveur d’images sur le LAN cité dans des NFO) et photos d’acteurs locales (`.actors/`, chemins absolus des NFO) servies par le cache
- [ ] T-099 Facettes et genres de l’accueil : premier calcul à froid ~170 ms sur 20 000 éléments (ensuite en cache) ; `TopGenres` compte aussi les éléments masqués par le contrôle parental (les compteurs, pas les éléments) ; précalcul à la fin d’un scan
- [ ] T-09B `agent.TestDiscovery` instable quand toute la suite tourne en parallèle (« impossible d'envoyer la requête de découverte », diffusion UDP) ; passe seul
- [ ] T-09C Images clés des fichiers distants sans Cues (TS, M2TS, MP4, MKV sans index) : scan par l'agent, près du fichier (endpoint à côté de `/probe`), résultat mis en cache en base ; aujourd'hui la vidéo est transcodée
- [x] T-09D Doublon du nom de dossier (« 21 Grams (2003) {tmdb-470}21 Grams (2003) {tmdb-470} ») : confirmé par `ls` sur la seedbox, le dossier porte réellement ce nom (renommage *arr/script côté utilisateur). Aether n'y est pour rien ; le parseur lit désormais une fois un nom de dossier *arr répété
- [ ] T-09E Lecture d'un agent dont la liaison est plus lente que le débit du fichier (remux Blu-ray à 30-40 Mbit/s) : aucune méthode ne tient le temps réel ; proposer une qualité réduite (transcodage) d'après le débit mesuré de l'agent
- [ ] T-09A Pagination par clé (keyset) au lieu d’OFFSET pour les très grandes bibliothèques (page 100 : ~70 ms à 20 000 éléments, croît linéairement)

## M8 — Fédération (v1.1)
- [ ] T-0D0 Appairage serveur ↔ serveur, bibliothèques partagées, droits par utilisateur
- [ ] T-0D1 Lecture relayée ou directe vers le serveur propriétaire
