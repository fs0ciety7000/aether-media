# Journal des modifications

Toutes les évolutions notables d'Aether sont consignées ici. Le format suit [Keep a Changelog](https://keepachangelog.com/fr/1.1.0/) et les versions suivent le [versionnage sémantique](https://semver.org/lang/fr/). Tant que la version majeure vaut 0, une version mineure peut contenir des changements incompatibles.

## [0.1.0] — à venir

Première version publique : la « Première séance ». Elle couvre le périmètre v1 décrit dans [PLAN.md](PLAN.md), hors validations sur matériel réel listées dans [TASKS.md](TASKS.md).

### Bibliothèques et analyse
- Bibliothèques de films, de séries, de musique et mixtes, composées d'une ou plusieurs **sources** : dossier local, **Agent Aether** installé sur une autre machine, serveur **SFTP** (seedbox).
- Reconnaissance des noms de fichiers selon les conventions Radarr, Sonarr, Lidarr, Kodi, Jellyfin et Plex : année, édition (`{edition-…}`), identifiants (`{tmdb-…}`, `[imdbid-…]`, `{tvdb-…}`), `SxxEyy`, épisodes multiples, séries quotidiennes, numérotation absolue, spéciaux, fichiers en plusieurs parties, disques et pistes. Corpus de plus de 600 noms testés.
- Analyse incrémentale (taille et date, sans lecture du contenu), surveillance des dossiers (fsnotify) et rescan périodique de secours, marqueurs d'exclusion (`.aetherignore`, `.ignore`, `.plexignore`, `.nomedia`), échantillons ignorés.
- Analyse ffprobe (pistes, langues, HDR, chapitres), sous-titres externes.
- Agent : appairage par code à usage unique, découverte sur le réseau local, partages gérés depuis l'interface, analyse sur place, notification des changements de fichiers, installation en service (`aether agent install`).

### Métadonnées
- Fichiers NFO (Kodi) et images locales en priorité, puis TMDB (films, séries, saisons, épisodes, distribution, collections) et MusicBrainz avec Cover Art Archive.
- Identification automatique avec score de confiance, écran « À identifier » et correction manuelle, champs verrouillés après une modification.
- Cache d'images local, variantes redimensionnées, blurhash et couleur dominante. Les photos des personnes passent aussi par ce cache.
- Langue et pays des métadonnées réglables, clé TMDB vérifiée en direct et stockée chiffrée.

### Parcourir et rechercher
- Accueil (hero, catégories, « Reprendre », « À suivre », récemment ajoutés, suggestions), bibliothèques avec filtres (genre, années, résolution, HDR, vu ou non vu), tris et défilement par curseur.
- Fiches détaillées : films, séries, saisons, épisodes, artistes et albums.
- Recherche instantanée plein texte (FTS5), insensible aux accents, résultats groupés par type.
- Collections manuelles et automatiques (TMDB), favoris, « Ma liste ».

### Lecture
- Décision automatique : lecture directe (HTTP Range), remux ou transcodage HLS fMP4 à la demande, selon les capacités détectées du navigateur.
- Avancer ou reculer n'importe où, choix des pistes audio et des sous-titres (WebVTT, ou incrustation des sous-titres image PGS / VobSub).
- Accélération matérielle optionnelle (VAAPI, QSV, NVENC, VideoToolbox) et conversion HDR vers SDR.
- Lecteur web : raccourcis clavier, plein écran, reprise, épisode suivant.

### Utilisateurs et synchronisation
- Comptes multiples (argon2id), sessions révocables, accès par bibliothèque, contrôle parental par âge (classification héritée des séries et saisons).
- Progression, vu ou non vu, notes, « Reprendre » et « À suivre » d'un appareil à l'autre.
- Synchronisation **Trakt** et **Simkl** par utilisateur : scrobbling en direct, historique dans les deux sens, points de reprise (Trakt), watchlist et notes. Les conflits sont résolus par la date la plus récente et consignés dans un journal.

### Administration, PWA et distribution
- Assistant de premier lancement : compte administrateur, langue et pays, clé TMDB, première bibliothèque.
- Réglages du serveur modifiables à chaud, file des tâches (relancer, annuler), journaux en direct avec secrets masqués, lectures en cours (arrêt possible par l'admin).
- Application web installable (PWA) avec écran hors ligne.
- Un seul binaire pour Linux (amd64, arm64, armv7), macOS et Windows, avec l'interface embarquée. Image Docker multi-arch avec jellyfin-ffmpeg, paquets `.deb` et `.rpm` (services systemd), cask Homebrew, archive Windows, commandes `aether service|agent install`.

### Sécurité et fiabilité
- Les URL d'images distantes (NFO, fournisseurs, redirections) ne peuvent atteindre ni le réseau local, ni la boucle locale, ni les adresses de métadonnées cloud. La vérification a lieu à la connexion, ce qui protège aussi du DNS rebinding. Seuls http et https sont acceptés, avec des délais et une taille maximale.
- Plus d'erreurs `database is locked` lorsqu'un rafraîchissement des métadonnées et une analyse tournent en même temps. Les transactions d'écriture passent par une connexion unique en `BEGIN IMMEDIATE`.
- Réponses JSON de l'API compressées (gzip) et interface servie précompressée (brotli, gzip).
- Parcours et recherche sous 100 ms par page sur une base de 20 000 éléments, contrôle parental compris (voir [docs/AUDIT-v0.1.0.md](docs/AUDIT-v0.1.0.md)).

### Limites connues
- Accélération matérielle, image arm64, service Windows et LaunchAgent macOS, ainsi que l'ajout à l'écran d'accueil iOS, n'ont pas encore été validés sur du matériel réel.
- La synchronisation Trakt et Simkl a été vérifiée contre la documentation officielle et des serveurs simulés. Il reste à l'essayer avec de vrais comptes.
- Le transcodage par l'agent, près des fichiers, n'est pas encore disponible : l'agent sert les fichiers et le serveur transcode.

[0.1.0]: https://github.com/fs0ciety7000/aether-media/releases/tag/v0.1.0
