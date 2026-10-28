# Guide d'Aether

Ce guide s'adresse à la personne qui installe et administre le serveur, comme à celles qui l'utilisent au quotidien. L'installation elle-même est décrite dans [INSTALL.md](INSTALL.md).

**Sommaire**

1. [Premier lancement](#1-premier-lancement)
2. [Bibliothèques et nommage des fichiers](#2-bibliothèques-et-nommage-des-fichiers)
3. [Sources distantes : agent et SFTP](#3-sources-distantes--agent-et-sftp)
4. [Métadonnées et identification](#4-métadonnées-et-identification)
5. [Lecture et transcodage](#5-lecture-et-transcodage)
6. [Utilisateurs et contrôle parental](#6-utilisateurs-et-contrôle-parental)
7. [Synchronisation Trakt et Simkl](#7-synchronisation-trakt-et-simkl)
8. [Application installable (PWA)](#8-application-installable-pwa)
9. [Sauvegarde et dépannage](#9-sauvegarde-et-dépannage)

---

## 1. Premier lancement

Ouvrez `http://<machine>:8096` dans un navigateur. Tant qu'aucun compte n'existe, Aether affiche l'**assistant de premier lancement**. Il compte cinq étapes ; toutes, sauf la première, peuvent être passées puis reprises plus tard.

1. **Compte administrateur** : un nom et un mot de passe d'au moins 8 caractères. Ce premier compte administre le serveur.
2. **Langue et pays des métadonnées** : la langue des titres et des résumés (par exemple `fr-FR`), et le pays dont on veut les classifications d'âge et les dates de sortie.
3. **Clé TMDB** : collez une clé API v3 ou un jeton de lecture v4, créés gratuitement sur [themoviedb.org/settings/api](https://www.themoviedb.org/settings/api). Aether la vérifie aussitôt et la stocke chiffrée. Sans clé, seuls les fichiers NFO et les images locales servent aux films et aux séries ; la musique utilise MusicBrainz, qui n'en demande pas.
4. **Première bibliothèque** : un nom, un type (films, séries, musique ou mixte) et une ou plusieurs sources (voir sections 2 et 3).
5. **Récapitulatif** : l'analyse démarre. Les éléments apparaissent au fil de l'eau ; affiches et résumés arrivent ensuite.

Si vous quittez l'assistant avant la fin, il reprend à la même étape à la prochaine connexion d'un administrateur. Tous ces réglages restent modifiables dans **Réglages › Serveur** et **Réglages › Bibliothèques**.

> Une valeur fournie par le fichier `aether.toml` ou par une variable `AETHER_*` est prioritaire : elle apparaît en lecture seule dans l'interface, avec la mention « Défini par la configuration ».

---

## 2. Bibliothèques et nommage des fichiers

### Créer une bibliothèque

**Réglages › Bibliothèques › Ajouter une bibliothèque**. Choisissez le type :

| Type | Contenu attendu |
|---|---|
| Films | un film par dossier, ou des fichiers à plat |
| Séries | un dossier par série, des sous-dossiers de saison facultatifs |
| Musique | `Artiste/Album/pistes` |
| Mixte | films et séries mélangés (préférez des bibliothèques séparées : la reconnaissance est plus fiable) |

Ajoutez ensuite une ou plusieurs **sources**. Pour un dossier du serveur, le sélecteur parcourt l'arborescence de la machine ; les dossiers système et temporaires (comme `/tmp`) ne sont pas proposés. En Docker, le dossier doit être monté dans le conteneur (par exemple `/media/films`).

Aether analyse la bibliothèque à sa création, puis :

- surveille les dossiers locaux et réanalyse quelques secondes après un ajout, un renommage ou une suppression ;
- relance une analyse périodique de sécurité (6 h par défaut, réglable dans **Réglages › Serveur**), utile pour les partages réseau qui ne signalent pas leurs changements ;
- peut être relancé à la main (**Analyser** sur la bibliothèque, ou **Réglages › Tâches › Analyse complète**).

Un fichier disparu est d'abord marqué manquant. Si sa source est hors ligne (disque démonté, agent éteint), rien n'est supprimé : les éléments restent visibles, grisés et marqués « Hors ligne ».

### Nommer les films

Aether comprend les conventions de Radarr, Kodi, Jellyfin et Plex, ainsi que les noms de « release ». Le plus sûr :

```
Films/
  Heat (1995)/
    Heat (1995).mkv
    Heat (1995).fr.srt
    Heat (1995).nfo                    # facultatif
    poster.jpg  fanart.jpg              # facultatif
  Blade Runner (1982) {edition-Final Cut}/
    Blade Runner (1982) {edition-Final Cut} [imdbid-tt0083658].mkv
  Kill Bill (2003)/
    Kill Bill (2003) - cd1.mkv
    Kill Bill (2003) - cd2.mkv
```

- **Année** entre parenthèses : elle départage les homonymes.
- **Édition** : `{edition-Director's Cut}` (Radarr et Plex), ou des mots reconnus comme « Director's Cut », « Extended », « Remastered ».
- **Identifiants** : `{tmdb-949}`, `[tmdbid-949]`, `{imdb-tt0113277}`, `[imdbid-tt0113277]`. Un identifiant dans le nom évite toute ambiguïté à l'identification.
- **Plusieurs parties** : `cd1`, `cd2`, `part1`, `disc 2`, `pt3` forment un seul film.
- **Bonus** : `-trailer`, `-featurette`, `-behindthescenes`, `-deleted`, `-interview`, `-scene`, `-short`, `-clip`, `-other`, ou les dossiers `Trailers/`, `Featurettes/`, `Behind The Scenes/`, `Deleted Scenes/`, `Extras/`… Ils ne sont pas pris pour des films.
- **Échantillons** (`sample`, dossier `Sample/`) : ignorés.

### Nommer les séries

Le format de Sonarr convient tel quel :

```
Séries/
  The Expanse (2015) [tvdbid-280619]/
    Season 01/
      The Expanse - S01E01 - Dulcinea.mkv
      The Expanse - S01E02E03 - The Big Empty.mkv
    Specials/
      The Expanse - S00E01 - Behind the Scenes.mkv
  Le Journal de 20h/
    Le Journal de 20h - 2024-03-15.mkv
```

- **Numérotation** : `S01E02`, `s01.e02`, `1x02`, `S01E02E03` ou `S01E02-E04` (épisodes multiples), « Saison 1 Épisode 2 ».
- **Saisons** : dossiers `Season 01`, `Saison 1`, `S01` ; `Specials` ou `Season 00` pour les spéciaux. Sans dossier de saison, le numéro vient du nom de fichier.
- **Émissions quotidiennes** : une date (`2024-03-15`, `2024.03.15`).
- **Anime** : la numérotation absolue (`[Groupe] Série - 123 [1080p].mkv`) et le format anime de Sonarr (`Série - S01E01 - 001 - Titre`) sont reconnus.
- **Identifiants** dans le nom du dossier de la série : `[tvdbid-…]`, `{tmdb-…}`, `[imdbid-…]`.

### Nommer la musique

```
Musique/
  Daft Punk/
    Discovery (2001)/
      01 - One More Time.flac
      02 - Aerodynamic.flac
      cover.jpg
  Pink Floyd/
    The Wall (1979)/
      Disc 1/01 - In the Flesh.flac
      Disc 2/01 - Hey You.flac
```

Les étiquettes des fichiers (artiste, album, numéro de piste) complètent ce que le chemin indique. Les disques se notent `Disc 1`, `CD2` ou `1-03 - Titre.flac`.

### Exclure des fichiers

Un fichier `.aetherignore`, `.ignore`, `.plexignore` ou `.nomedia` dans un dossier exclut ce dossier et tout son contenu. Les fichiers et dossiers cachés (commençant par un point) ne sont jamais analysés.

---

## 3. Sources distantes : agent et SFTP

Une bibliothèque peut réunir des fichiers de plusieurs machines. Le serveur garde la base, les métadonnées et l'interface ; les fichiers restent où ils sont.

### Agent Aether (une machine que vous contrôlez)

L'agent est le même binaire, lancé en mode `aether agent`. Installation : voir [INSTALL.md](INSTALL.md#installer-lagent-sur-une-machine-distante). Il n'a ni base de données ni configuration.

1. Sur le serveur : **Réglages › Bibliothèques**, ajoutez une source **Agent Aether** à une bibliothèque, ou **Appairer un agent**.
2. Les agents du réseau local apparaissent d'eux-mêmes (découverte UDP, port 7360). Sinon, saisissez son adresse, par exemple `http://192.168.1.20:8097`.
3. Tapez le **code d'appairage** affiché par l'agent : dans ses journaux, ou sur `http://127.0.0.1:8097` depuis la machine de l'agent. Le code est à usage unique et change régulièrement.
4. Parcourez les dossiers de la machine et choisissez ceux à partager.

L'agent analyse les fichiers sur place (ffprobe) et prévient le serveur de chaque changement : pas besoin de rescan périodique. Les fichiers et dossiers cachés de l'agent ne sont jamais exposés, même si vous partagez un dossier personnel.

Hors du réseau local, passez par un VPN (WireGuard, Tailscale) ou activez TLS sur l'agent (voir [AGENT_PROTOCOL.md](AGENT_PROTOCOL.md)).

### SFTP (seedbox, NAS sans installation)

Ajoutez une source **SFTP** : hôte, port, utilisateur, puis mot de passe ou clé privée (avec sa phrase de passe, le cas échéant), et le dossier distant.

À la première connexion, Aether affiche l'**empreinte de la clé d'hôte** (`SHA256:…`). Vérifiez-la auprès de votre hébergeur avant de la confirmer : elle est ensuite exigée à chaque connexion, ce qui protège contre l'usurpation du serveur. Les secrets (mots de passe, clés, jetons) sont chiffrés dans la base et ne sont jamais réaffichés.

Les sources SFTP sont analysées périodiquement : un fichier ajouté sur la seedbox apparaît au rescan suivant, ou tout de suite avec **Analyser**.

### État des sources

Chaque source affiche son état (en ligne, hors ligne) et sa dernière erreur. **Tester** vérifie la connexion avec les identifiants enregistrés. Une source hors ligne ne fait rien disparaître ; ses éléments reviennent dès qu'elle est de nouveau joignable.

---

## 4. Métadonnées et identification

### D'où viennent les informations

Pour chaque élément, Aether consulte dans l'ordre :

1. les **fichiers locaux** : NFO au format Kodi (`movie.nfo`, `<nom du fichier>.nfo`, `tvshow.nfo`, `season.nfo`…) et images (`poster.jpg`, `fanart.jpg`, `logo.png`, `banner.jpg`, `<nom>-thumb.jpg`, dossier `extrafanart/`…) ;
2. **TMDB** pour les films et les séries (titres, résumés, distribution, classifications, collections, images) ;
3. **MusicBrainz** et **Cover Art Archive** pour la musique.

Un NFO a toujours priorité. Les images sont copiées dans le cache local du serveur, redimensionnées à la demande : l'interface ne charge jamais d'image depuis un site tiers. Les photos des acteurs passent elles aussi par ce cache.

Par sécurité, les adresses d'images indiquées dans un NFO ne peuvent pas viser le réseau local ni la machine elle-même (127.0.0.1, 192.168.x.x, adresses de métadonnées cloud…). Seuls http et https sont acceptés. Placez plutôt l'image à côté du fichier.

### Identification

Aether identifie un élément par les identifiants présents dans son nom ou son NFO, sinon par une recherche titre + année. Il n'accepte automatiquement qu'un résultat net. Les autres éléments sont marqués **À identifier** :

- la page **À identifier** (menu de la barre latérale, administrateurs) les regroupe ;
- **Identifier…** (menu d'une fiche) propose des candidats ; corrigez le titre ou l'année si besoin, puis choisissez le bon.

### Corrections et verrouillage

**Modifier…** sur une fiche change le titre, le titre original, le résumé, l'accroche, l'année ou la classification. Un champ modifié est **verrouillé** : les rafraîchissements suivants ne l'écrasent plus. On peut le déverrouiller depuis la même boîte de dialogue.

### Rafraîchir

- **Actualiser les métadonnées** sur une fiche relit les sources pour cet élément.
- **Réglages › Tâches › Rafraîchir les métadonnées** le fait pour toute la médiathèque. Faites-le après un changement de langue ou de pays des métadonnées : les titres déjà identifiés ne sont traduits qu'à ce moment.

---

## 5. Lecture et transcodage

### Comment Aether choisit

Au lancement d'une lecture, le navigateur indique ce qu'il sait lire. Aether choisit alors :

| Méthode | Quand | Coût |
|---|---|---|
| **Lecture directe** | conteneur et codecs lisibles par le navigateur | nul : le fichier est servi tel quel |
| **Remux** | codecs lisibles mais conteneur non pris en charge (MKV sur Safari, par exemple) | faible : FFmpeg recopie les flux en HLS sans les réencoder |
| **Transcodage** | codec vidéo ou audio illisible, débit trop élevé, HDR sur un écran SDR, sous-titres image à incruster | élevé : FFmpeg réencode en H.264 / AAC |

Le panneau **i** du lecteur (touche `i`) affiche la méthode retenue et ses raisons. Les administrateurs voient toutes les lectures en cours dans **Réglages › Lectures**, et peuvent les arrêter.

### Sous-titres

Les sous-titres texte (SRT, ASS, WebVTT, internes ou externes comme `Film.fr.srt`) sont convertis en WebVTT et affichés par le lecteur, sans transcodage. Les sous-titres image (PGS des Blu-ray, VobSub des DVD) doivent être incrustés dans la vidéo, ce qui déclenche un transcodage.

Dans **Profil**, chaque utilisateur choisit sa langue audio et de sous-titres préférée, et le mode d'affichage des sous-titres (automatique, toujours, forcés seulement, désactivés).

### Raccourcis du lecteur

| Touche | Action |
|---|---|
| Espace, `k` | lecture / pause |
| ← / → , `j` / `l` | reculer / avancer de 10 s |
| ↑ / ↓ | volume |
| `m` | couper le son |
| `f` | plein écran |
| `c` | changer de sous-titres |
| `n` | épisode suivant |
| `0` à `9` | aller à 0 %, 10 %… 90 % |
| `i` | informations de lecture |
| Échap | fermer le panneau, quitter le plein écran |

### Limiter la charge

**Réglages › Serveur › Transcodages simultanés** (2 par défaut) borne le nombre de processus FFmpeg. Au-delà, une nouvelle lecture qui nécessite un transcodage est refusée avec un message ; les lectures directes ne sont jamais limitées.

### Accélération matérielle

Elle décharge le processeur pendant les transcodages. Choisissez le mode dans **Réglages › Serveur** (ou `AETHER_HWACCEL`) ; il s'applique au **prochain redémarrage** du serveur.

| Mode | Matériel | Prérequis |
|---|---|---|
| `vaapi` | Intel, AMD (Linux) | accès à `/dev/dri/renderD128` (groupe `render`) |
| `qsv` | Intel Quick Sync (Linux) | idem, pilote iHD |
| `nvenc` | NVIDIA | pilote NVIDIA, et nvidia-container-toolkit en Docker |
| `videotoolbox` | macOS | aucun |
| `auto` | essaie les modes disponibles | |

**Réglages › Serveur** affiche les encodeurs matériels qui fonctionnent réellement sur la machine (un test d'encodage est fait pour chacun ; **Détecter le matériel** relance ce test). La configuration Docker (`/dev/dri`, `group_add`, NVIDIA) est détaillée dans [INSTALL.md](INSTALL.md#accélération-matérielle).

Le build `jellyfin-ffmpeg7`, inclus dans l'image Docker, est recommandé ailleurs aussi : il apporte la conversion HDR → SDR fidèle (`zscale`) et l'incrustation des sous-titres ASS (`libass`).

---

## 6. Utilisateurs et contrôle parental

### Comptes

**Réglages › Utilisateurs › Ajouter** : nom, mot de passe, rôle (administrateur ou membre). Chaque utilisateur a sa propre progression, ses favoris, ses notes, sa liste « Ma liste » et ses connexions Trakt et Simkl.

| | Membre | Administrateur |
|---|---|---|
| Parcourir, rechercher, lire | oui | oui |
| Progression, favoris, notes, Ma liste, Trakt / Simkl | les siens | les siens |
| Bibliothèques, sources, analyses | non | oui |
| Identifier ou modifier un élément, voir les chemins des fichiers | non | oui |
| Utilisateurs, réglages, tâches, journaux, lectures en cours | non | oui |

Dans **Profil**, chacun change son mot de passe et voit ses sessions ouvertes (navigateurs, appareils), qu'il peut fermer une à une. Changer de mot de passe ferme les autres sessions. Après 10 échecs de connexion en 15 minutes, une adresse IP est temporairement bloquée.

### Accès par bibliothèque

Dans la fiche d'un utilisateur, décochez **Toutes les bibliothèques** pour choisir celles qu'il voit. Les autres disparaissent partout : listes, recherche, accueil, et une adresse directe répond « introuvable ».

### Contrôle parental

Choisissez une **classification maximale** : tous publics, 6, 10, 12, 16 ou 18 ans. Les classifications de tous les pays (CSA en France, MPAA aux États-Unis, FSK…) sont ramenées à un âge. Un épisode hérite de la classification de sa série et de sa saison.

L'option **Masquer les contenus non classés** cache aussi ce qui n'a pas de classification, ce qui est plus prudent pour un enfant. La musique n'est pas concernée.

---

## 7. Synchronisation Trakt et Simkl

Chaque utilisateur peut relier **son** compte Trakt et / ou Simkl. Aether synchronise alors :

| | Trakt | Simkl |
|---|---|---|
| Scrobbling en direct (« en cours de lecture », puis vu) | oui | oui |
| Historique vu / non vu, dans les deux sens | oui | oui |
| Points de reprise | oui | pas encore |
| Watchlist ↔ « Ma liste » | oui | oui (« Plan to Watch ») |
| Notes | oui | oui |

La correspondance se fait par identifiants (TMDB, IMDb, TVDB) : un élément non identifié, ou absent de la bibliothèque, est ignoré.

### Étape 1 : créer l'application (administrateur, une fois)

Aether ne fournit pas d'identifiants d'application : chaque serveur crée les siens, gratuitement.

**Trakt**

1. Connectez-vous sur [trakt.tv](https://trakt.tv), puis ouvrez [trakt.tv/oauth/applications](https://trakt.tv/oauth/applications) › **New Application**.
2. Nom : par exemple « Aether (maison) ». **Redirect uri** : `urn:ietf:wg:oauth:2.0:oob`. Laissez les autres champs vides.
3. Enregistrez, puis copiez le **Client ID** et le **Client Secret**.

**Simkl**

1. Connectez-vous sur [simkl.com](https://simkl.com), puis ouvrez [simkl.com/settings/developer](https://simkl.com/settings/developer/) › **Create new app**.
2. Donnez-lui un nom ; pour l'adresse de redirection, `urn:ietf:wg:oauth:2.0:oob` convient (Aether utilise l'appairage par code PIN).
3. Copiez le **Client ID** (le secret n'est pas nécessaire).

Saisissez ensuite ces identifiants dans **Réglages › Synchronisation**. Ils sont stockés chiffrés. Vous pouvez aussi les fournir par la configuration (`AETHER_TRAKT_CLIENT_ID`, `AETHER_TRAKT_CLIENT_SECRET`, `AETHER_SIMKL_CLIENT_ID`, ou la section `[sync]` d'`aether.toml`) : ils sont alors prioritaires et en lecture seule dans l'interface.

### Étape 2 : relier son compte (chaque utilisateur)

1. **Profil › Synchronisation › Connecter**, sur Trakt ou Simkl.
2. Aether affiche un code. Ouvrez [trakt.tv/activate](https://trakt.tv/activate) ou [simkl.com/pin](https://simkl.com/pin), sur n'importe quel appareil, et saisissez-le.
3. Dès la validation, l'import initial démarre.

Ensuite, Aether synchronise toutes les 30 minutes, environ une minute après chaque changement local (vu, note, Ma liste, progression) et à la demande (**Synchroniser maintenant**). Chaque fonction se désactive séparément.

### Conflits

Si un même élément a changé des deux côtés depuis la dernière synchronisation, **la date la plus récente l'emporte**. Le conflit est consigné dans le **journal** de la page Synchronisation, avec le côté retenu.

**Déconnecter** révoque l'accès chez Trakt et efface l'état de synchronisation ; vos données dans Aether (vus, notes, Ma liste) sont conservées.

---

## 8. Application installable (PWA)

Aether s'installe comme une application, sans magasin :

- **Android (Chrome, Edge)** et **ordinateur (Chrome, Edge)** : Aether propose discrètement de l'installer ; sinon, menu du navigateur › **Installer l'application**.
- **iPhone et iPad (Safari)** : bouton Partager › **Sur l'écran d'accueil**.

L'application s'ouvre en plein écran, sans barre d'adresse, et se met à jour toute seule au lancement suivant une mise à jour du serveur. Elle a besoin du serveur pour fonctionner : sans réseau, elle affiche « Vous êtes hors ligne » et un bouton **Réessayer**. Aucun média n'est conservé sur l'appareil.

Pour l'installer depuis l'extérieur de votre réseau, le serveur doit être servi en **HTTPS** : les navigateurs n'autorisent les PWA qu'en HTTPS, ou sur `localhost`. Placez Aether derrière un reverse proxy (Caddy, Traefik, nginx) ou un VPN.

---

## 9. Sauvegarde et dépannage

### Sauvegarde

Tout l'état du serveur tient dans son dossier de données : la base `aether.db` (bibliothèques, métadonnées, comptes, progression), `secret.key` (sans elle, les secrets stockés sont perdus) et `server.id`. Le cache d'images se reconstruit. La procédure complète, avec la restauration, est dans [INSTALL.md › Sauvegarde](INSTALL.md#sauvegarde).

### Où regarder en cas de problème

- **Réglages › Journaux** : les derniers messages du serveur, filtrables par niveau, avec suivi en direct et téléchargement. Les secrets y sont masqués : vous pouvez joindre ce fichier à un rapport de bug.
- **Réglages › Tâches** : analyses, rafraîchissements et synchronisations en cours ou en échec, avec leur erreur. Une tâche en échec peut être relancée.
- **Réglages › Serveur** : version, chemins, FFmpeg détecté, accélération matérielle disponible.
- Journaux système : `docker compose logs -f aether`, `journalctl -u aether -f`, ou `C:\ProgramData\Aether\logs\aether.log`.
- Plus de détails : `AETHER_LOG_LEVEL=debug`.

### Questions fréquentes

**Un film est mal reconnu, ou reste « À identifier ».**
Ajoutez l'année au nom du dossier, ou mieux un identifiant (`{tmdb-…}`, `[imdbid-…]`), puis **Actualiser les métadonnées**. Sinon, **Identifier…** sur la fiche.

**Les affiches n'apparaissent pas.**
Vérifiez qu'une clé TMDB est enregistrée (**Réglages › Serveur**) et que le serveur accède à Internet. Une erreur réseau ou une clé refusée apparaît dans **Réglages › Journaux**. Une image locale (`poster.jpg`) est toujours prise en priorité.

**Une image indiquée dans mon NFO est ignorée.**
Si son adresse vise le réseau local (`http://192.168.…`, `http://nas.local/…`) ou la machine elle-même, elle est refusée par sécurité. Enregistrez l'image à côté du fichier vidéo, sous le nom `poster.jpg` ou `fanart.jpg`.

**Un fichier ajouté n'apparaît pas.**
Sur un partage réseau (SMB, NFS) ou une source SFTP, les changements ne sont pas toujours signalés : lancez **Analyser** sur la bibliothèque, ou attendez le rescan périodique. Vérifiez aussi qu'aucun `.aetherignore` ne se trouve dans un dossier parent, et que le compte du service peut lire le fichier.

**La lecture transcode alors que le fichier est « simple ».**
Le panneau **i** du lecteur donne la raison : codec audio (DTS, TrueHD), sous-titres image activés, débit au-dessus de la limite choisie dans le lecteur… Désactiver les sous-titres PGS ou choisir une autre piste audio suffit souvent à revenir en lecture directe.

**« Trop de lectures en cours sur le serveur ».**
La limite de **Réglages › Serveur** est atteinte. Augmentez-la si la machine le permet, ou activez l'accélération matérielle.

**L'accélération matérielle ne s'active pas.**
Elle n'est prise en compte qu'au redémarrage. **Réglages › Serveur** liste les encodeurs détectés : si la liste est vide, le serveur n'accède pas au GPU (en Docker : `devices: /dev/dri` et `group_add` avec le groupe `render` de l'hôte ; sur Linux : appartenance du compte `aether` aux groupes `render` et `video`).

**Une source est « hors ligne ».**
Pour un agent : la machine est-elle allumée, et le port 8097 ouvert ? **Tester** donne l'erreur exacte. Après une erreur réseau, Aether cesse d'appeler l'agent et ne le sonde plus qu'une fois après 1, 2, 5, 15 puis toutes les 30 minutes (le message indique l'heure du prochain essai) ; les analyses en attente reprennent seules quand il répond. Un hébergeur de seedbox peut bloquer une adresse qui a ouvert trop de connexions : patientez, ou passez par un tunnel SSH (voir l'installation, « Seedbox sans root »). Pour SFTP : identifiants, empreinte de l'hôte (une seedbox réinstallée change de clé, il faut alors la confirmer de nouveau).

**L'appairage d'un agent échoue.**
Le code change régulièrement et ne sert qu'une fois : relisez-le dans les journaux de l'agent ou sur `http://127.0.0.1:8097`. Après plusieurs codes faux, l'agent impose une courte pause. Si la découverte ne trouve rien, le réseau bloque peut-être la diffusion UDP (port 7360) : saisissez l'adresse à la main.

**Trakt ou Simkl indique « reconnexion nécessaire ».**
Le jeton a expiré ou a été révoqué : **Profil › Synchronisation › Reconnecter**. Si l'erreur concerne tous les utilisateurs, vérifiez les identifiants d'application dans **Réglages › Synchronisation**.

**J'ai oublié le mot de passe administrateur.**
Un autre administrateur peut le réinitialiser dans **Réglages › Utilisateurs**. Créer un second compte administrateur dès l'installation est donc une bonne précaution : en v0.1.0, il n'existe pas encore de commande de réinitialisation hors de l'interface.

**`database is locked` dans les journaux.**
Ce message ne devrait plus apparaître depuis la v0.1.0. S'il revient, joignez les journaux à un rapport de bug, en indiquant les tâches en cours à ce moment (**Réglages › Tâches**).
