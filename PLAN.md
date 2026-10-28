# Aether — Plan (draft v0.1)

> Statut : **en attente de validation** de la stack (voir `ARCHITECTURE.md` §2).
> Chaque jalon est livrable et démontrable. On ne passe au suivant qu'une fois les critères de sortie remplis.

## Vision
Un serveur multimédia self-hosted, **beau, calme et fiable** : pointer ses dossiers (*arr-friendly), et retrouver en quelques minutes une bibliothèque digne d'une salle privée, lisible partout.

## Périmètre v1 (MVP « Première séance »)
**Dedans** : films et séries · musique (base) · scan + watcher · métadonnées TMDB / NFO / MusicBrainz · UI web + thèmes · direct play + remux + transcode HLS · **sources distantes (agent Aether, SFTP) — plusieurs serveurs par bibliothèque** · progression / Reprendre / À suivre · **synchronisation Trakt / Simkl** · collections · multi-utilisateurs · recherche et filtres · Docker et binaires Linux/macOS/Windows · PWA.

**Hors v1** : photos (v1.1) · fédération entre serveurs Aether complets (v1.1, M8) · Live TV / DVR · SyncPlay · plugins tiers · apps natives TV · compatibilité avec l'API Jellyfin · accès distant relayé.

---

## Jalons

### M0 — Fondations (≈ 1 semaine)
Repo, CI, squelette Go + SvelteKit, config, SQLite + migrations, logs, `aether serve` qui sert une page.
**Sortie** : `docker run` → page d'accueil vide stylée avec les tokens ; CI verte.

### M1 — Design system & shell UI (≈ 1–2 semaines, en parallèle de M2)
Tokens, 5 thèmes, typographies, composants de base (boutons, cartes, rangée, hero, champs), app shell, page de démo `/design`.
**Sortie** : galerie de composants validée visuellement par le PO, contrastes AA vérifiés.

### M2 — Scan & indexation (≈ 2 semaines)
Package `naming` (films, séries, musique, conventions *arr), walker, resolvers, fingerprint incrémental, watcher fsnotify, ffprobe, jobs, SSE de progression.
**Sortie** : une bibliothèque de test (≥ 500 fichiers aux noms variés) est indexée correctement à ≥ 98 % ; un rescan sans changement prend < 5 s.

### M3 — Métadonnées & images (≈ 2 semaines)
Orchestrateur de providers, NFO, TMDB (films/séries), MusicBrainz + CAA, cache d'images, blurhash/couleur dominante, « à identifier » + correction manuelle, champs verrouillés.
**Sortie** : affiches, synopsis et casting présents pour ≥ 95 % du jeu de test ; aucune requête externe sans cache.

### M4 — Parcourir & rechercher (≈ 2 semaines)
API items/home/search, pages Accueil, Bibliothèque (filtres + tri + pagination par curseur), Détail, Recherche FTS5.
**Sortie** : navigation fluide (< 100 ms par page API pour 20 k items), recherche instantanée.

### M5 — Lecture (≈ 3 semaines)
Profils de capacités navigateur, décision direct / remux / transcode, HLS fMP4 à la demande, seek, sous-titres VTT et burn-in, sélection des pistes, lecteur stylé, accélération matérielle (VAAPI/NVENC/QSV/VideoToolbox) en option.
**Sortie** : la matrice de test (MKV H.264/HEVC/AV1, AAC/AC3/DTS/TrueHD, SRT/ASS/PGS) passe sur Chrome, Firefox et Safari (desktop et iOS).

### M5b — Sources distantes (≈ 2–3 semaines) — *ajouté le 2026-10-28 à la demande du PO*
Les fichiers vivent majoritairement sur des machines distantes. Une bibliothèque devient un ensemble de **sources** :
`local` (dossier), `agent` (Agent Aether installé sur une machine que l'on contrôle), `sftp` (seedbox, sans installation).
- Abstraction de système de fichiers (VFS) utilisée par le scanner, ffprobe, la lecture directe et FFmpeg (les sources non locales sont exposées à FFmpeg en HTTP avec Range).
- **Agent Aether** : même binaire (`aether agent`), appairage par jeton, API fichiers (listing récursif, Range), analyse ffprobe **sur place**, transcodage **près des fichiers** (moins de bande passante), notifications de changements (pas d'inotify à distance).
- **SFTP** : connexion par clé ou mot de passe, listing, lecture par plages, reconnexion ; scan périodique.
- UI : ajout d'une source par type, test de connexion, état (en ligne / hors ligne), les éléments d'une source hors ligne restent visibles mais grisés.
**Sortie** : une bibliothèque « Films » combinant un dossier local, un agent et une seedbox SFTP ; scan, métadonnées et lecture (directe, remux, transcodage) fonctionnent pour les trois.

### M6 — Utilisateurs & progression (≈ 1–2 semaines)
Comptes, sessions, accès par bibliothèque, contrôle parental, progression, vu/non vu, favoris, Reprendre, À suivre, collections manuelles et automatiques TMDB.
**Sortie** : 3 utilisateurs avec des historiques indépendants ; la reprise fonctionne d'un appareil à l'autre.

### M6b — Synchronisation Trakt / Simkl (≈ 1–2 semaines) — *ajouté le 2026-10-28*
Par utilisateur, connexion OAuth (Trakt : device code ; Simkl : code PIN), correspondance par identifiants TMDB/IMDb/TVDB déjà stockés.
- **Scrobbling en direct** (début / pause / fin de lecture, depuis les pings du lecteur).
- **Historique « vu » dans les deux sens** (import initial puis synchronisation incrémentale).
- **Progression de reprise** (Trakt playback progress) dans les deux sens.
- **Watchlist et notes**.
- Règles de conflit (la date la plus récente gagne), file de jobs avec reprise, respect des limites de débit.
**Sortie** : regarder un épisode dans Aether le marque vu sur Trakt et Simkl ; un film marqué vu ou noté sur Trakt l'est dans Aether.

### M7 — Finition & distribution (≈ 2 semaines)
Assistant de premier lancement, admin (tâches, journaux, transcodage), PWA, paquets (Docker multi-arch, .deb/.rpm, Homebrew, zip Windows), documentation, audit accessibilité et performance.
**Sortie** : release `v0.1.0` installable en < 5 min sur les 3 OS.

### M8 — Fédération (v1.1)
Relier plusieurs serveurs Aether complets : appairage serveur à serveur, bibliothèques partagées (lecture seule) avec droits par utilisateur, lecture relayée ou directe vers le serveur qui possède le fichier, partage entre amis.

```
Semaines  1   2   3   4   5   6   7   8   9  10  11  12  13  14  15  16  17  18  19  20
M0        ███
M1            ██████
M2            ████████
M3                    ████████
M4                            ████████
M5                                    ████████████
M5b                                                     ██████████
M6                                                                ██████
M6b                                                                     ██████
M7                                                                           ████████
```

## Gouvernance
- **PO / chef de projet** (moi) : maintient ce plan, `TASKS.md`, valide les livrables.
- **Agents** : exécution par tâche, une branche ou PR par tâche, revue avant merge.
- **Toi** : validation de la stack, validation visuelle du design system (M1), arbitrages de périmètre.
- Chaque jalon se termine par une **démo** + une mise à jour de ces documents.

## Risques
| Risque | Mitigation |
|---|---|
| Matrice de compatibilité navigateur et codecs | profils testés automatiquement (Playwright + fichiers échantillons), fallback transcode H.264/AAC systématique |
| Quotas et disponibilité TMDB/TVDB | cache agressif, NFO prioritaire, rate limiting, clé personnelle possible |
| Performances du scan sur NAS (SMB/NFS) | fingerprint sans lecture du contenu, scan périodique en plus du watcher, concurrence bornée |
| Dérive du périmètre | v1 figée ci-dessus ; tout ajout passe en v1.x |
| FFmpeg selon les plateformes | version minimale vérifiée au démarrage, image Docker avec jellyfin-ffmpeg ou ffmpeg statique |
