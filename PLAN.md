# Aether — Plan (draft v0.1)

> Statut : **en attente de validation** de la stack (voir `ARCHITECTURE.md` §2).
> Chaque jalon est livrable et démontrable. On ne passe au suivant qu'une fois les critères de sortie remplis.

## Vision
Un serveur multimédia self-hosted, **beau, calme et fiable** : pointer ses dossiers (*arr-friendly), et retrouver en quelques minutes une bibliothèque digne d'une salle privée, lisible partout.

## Périmètre v1 (MVP « Première séance »)
**Dedans** : films et séries · musique (base) · scan + watcher · métadonnées TMDB / NFO / MusicBrainz · UI web + thèmes · direct play + remux + transcode HLS · progression / Reprendre / À suivre · collections · multi-utilisateurs · recherche et filtres · Docker et binaires Linux/macOS/Windows · PWA.

**Hors v1** : photos (v1.1) · Live TV / DVR · SyncPlay · plugins tiers · apps natives TV · compatibilité avec l'API Jellyfin · accès distant relayé.

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

### M6 — Utilisateurs & progression (≈ 1–2 semaines)
Comptes, sessions, accès par bibliothèque, contrôle parental, progression, vu/non vu, favoris, Reprendre, À suivre, collections manuelles et automatiques TMDB.
**Sortie** : 3 utilisateurs avec des historiques indépendants ; la reprise fonctionne d'un appareil à l'autre.

### M7 — Finition & distribution (≈ 2 semaines)
Assistant de premier lancement, admin (tâches, journaux, transcodage), PWA, paquets (Docker multi-arch, .deb/.rpm, Homebrew, zip Windows), documentation, audit accessibilité et performance.
**Sortie** : release `v0.1.0` installable en < 5 min sur les 3 OS.

```
Semaines  1   2   3   4   5   6   7   8   9  10  11  12  13  14
M0        ███
M1            ██████
M2            ████████
M3                    ████████
M4                            ████████
M5                                    ████████████
M6                                                ██████
M7                                                      ████████
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
