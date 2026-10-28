# Audit accessibilité et performance — v0.1.0

Audit de la tâche T-084, fait le 28 octobre 2026 sur la branche de la release, avant et après correctifs.

## Méthode

**Instance réelle.** Le binaire `make build` (interface embarquée) part d'un dossier de données vide, avec la bibliothèque de démonstration de `web/scripts/demo-library.sh` (13 films, 2 séries, 3 albums de 2 artistes, sans TMDB). Le script d'audit suit le vrai parcours :

1. page de création du compte (assistant, étape 1) ;
2. `POST /api/v1/setup`, puis l'assistant repris à l'étape « métadonnées » ;
3. création des bibliothèques Films, Séries et Musique, attente de la fin des analyses, assistant terminé ;
4. accueil, bibliothèque Films, fiche d'un film, recherche (`?q=mar`), lecteur, Réglages › Serveur, Profil ;
5. page de connexion, sans session.

**axe-core 4.14** (celui de Lighthouse), injecté par Playwright dans Chrome for Testing 153, avec les règles WCAG 2.0 / 2.1 / 2.2 A et AA et les bonnes pratiques. Desktop : 1440 × 900. Mobile : profil Pixel 7 de Playwright.

**Lighthouse 12.8.2** (`npx`, Chromium sans interface, `--no-sandbox`), catégories performance, accessibilité et bonnes pratiques. Desktop : `--preset=desktop`. Mobile : réglages par défaut, soit réseau 4G lente simulée et processeur ralenti ×4. La session est passée par l'en-tête `Cookie`.

**Machine.** Conteneur à 4 cœurs partagés, sans GPU. Les scores de performance mobile varient d'environ ±10 points d'une passe à l'autre, surtout le TBT. Les tendances sont fiables, pas les unités.

Les scripts (`audit.cjs`, `profile.cjs`) ne sont pas versionnés. Ils tiennent en une centaine de lignes et pourront rejoindre `web/scripts/` si on veut rejouer l'audit en CI.

## Résultats

### Avant

| Page | A11y D | A11y M | Perf D | Perf M | Bonnes pratiques D / M | LCP M | TBT M | Poids M | axe serious + critical D / M |
|---|---|---|---|---|---|---|---|---|---|
| Assistant, compte | 100 | 100 | 98 | 74 | 100 / 100 | 4,7 s | 120 ms | 401 Kio | 0 / 0 |
| Assistant | 100 | 100 | 97 | 72 | 100 / 100 | 4,8 s | 150 ms | 403 Kio | 0 / 0 |
| Accueil | 96 | 100 | 96 | 46 | 100 / 100 | 5,3 s | 1 220 ms | 517 Kio | 2 / 0 |
| Bibliothèque | 96 | 100 | 97 | 64 | 100 / 100 | 4,4 s | 500 ms | 423 Kio | 2 / 0 |
| Fiche | 97 | 100 | 95 | 60 | 100 / 100 | 5,9 s | 370 ms | 410 Kio | 2 / 0 |
| Recherche | 96 | 100 | 99 | 76 | 100 / 100 | 3,8 s | 330 ms | 277 Kio | 2 / 0 |
| Lecteur | 100 | 100 | 95 | 64 | 100 / 100 | 4,6 s | 460 ms | 794 Kio | 0 / 0 |
| Réglages | 97 | 100 | 98 | 68 | 100 / 100 | 4,2 s | 460 ms | 299 Kio | 2 / 0 |
| Profil | 96 | 100 | 99 | 73 | 100 / 100 | 4,1 s | 320 ms | 285 Kio | 2 / 0 |
| Connexion | 100 | 100 | 99 | 83 | 96 / 96 | 3,5 s | 180 ms | 253 Kio | 0 / 0 |

D : desktop, M : mobile.

Violations axe relevées, toutes en desktop :

- `color-contrast` (serious), sur 6 pages : le titre de section « Bibliothèques » de la barre latérale utilisait `--text-faint` sur `--bg`, soit 3,83:1 pour un texte de 12 px, alors que 4,5:1 est requis.
- `label-content-name-mismatch` (serious, WCAG 2.5.3), sur 6 pages : le bouton du menu utilisateur affichait « admin Administrateur » mais s'appelait « Compte de admin (Administrateur) ».

Constats Lighthouse :

- **Aucune compression HTTP**, ni pour l'interface (JS, CSS, HTML) ni pour le JSON de l'API (`/api/v1/home` : 34 Kio non compressés). C'était le premier poste de poids et de FCP en mobile.
- **Image LCP chargée en différé** (`loading="lazy"`) sur la bibliothèque et la recherche.
- **TBT mobile élevé** sur l'accueil (1,2 s). Le profil CPU (`Profiler` CDP, CPU ×4) désignait trois causes : chaque rangée lisait `scrollLeft` et `scrollWidth` pendant son montage, ce qui forçait un reflow complet par rangée ; l'en-tête lisait `window.scrollY` au montage (autre reflow forcé, visible sur toutes les pages) ; le décodage blurhash produisait des PNG de 32 px.
- **Images du hero** : les quatre diapositives cachées téléchargeaient leur fond, avec un attribut `sizes` trop large.
- **Erreur console sur la page de connexion** : `GET /api/v1/me` répondait 401, ce qui coûtait 4 points de bonnes pratiques.

### Correctifs

| Constat | Correctif |
|---|---|
| Contraste de « Bibliothèques » | `--text-muted` (AA) au lieu de `--text-faint` (`Sidebar.svelte`) |
| Nom du menu utilisateur | `Menu` accepte `labelFromContent` : le bouton prend son nom de son texte visible, `label` ne nomme plus que le menu |
| Compression | build `precompress` (brotli et gzip) servi selon `Accept-Encoding` par `internal/webui`, sauf pour une requête `Range` ; gzip à la volée du JSON de `/api/` (`internal/api/compress.go`), jamais pour SSE, médias, HLS, sous-titres ni images |
| LCP différé | les 6 premières cartes d'une grille et les 4 premiers résultats de recherche sont chargés tout de suite (`eager`, `fetchpriority=high`) |
| Reflows forcés | `Row` et `AppHeader` mesurent à l'image suivante (`requestAnimationFrame`, `ResizeObserver`), jamais pendant le montage |
| Blurhash | décodé sur 16 px au lieu de 32 (4 fois moins de calcul et d'encodage PNG ; l'image est floutée de toute façon) |
| Hero | seules la diapositive affichée et la suivante chargent leur image ; `sizes` passe de 66vw à 52vw |
| 401 en console | `GET /system/info` indique `authenticated` ; sans session, l'interface ne demande plus `/me` |
| Photos des personnes | servies par le cache (T-047) à la taille affichée (`&w=`), avec chargement différé et repli sur les initiales |

### Après

| Page | A11y D | A11y M | Perf D | Perf M | Bonnes pratiques D / M | LCP M | TBT M | Poids M | axe serious + critical D / M |
|---|---|---|---|---|---|---|---|---|---|
| Assistant, compte | 100 | 100 | 99 | 82 | 100 / 100 | 3,7 s | 170 ms | 147 Kio | 0 / 0 |
| Assistant | 100 | 100 | 99 | 85 | 100 / 100 | 3,4 s | 170 ms | 148 Kio | 0 / 0 |
| Accueil | **100** | 100 | 93 | 55 – 62 | 100 / 100 | 4,0 – 4,3 s | 990 – 1 640 ms | 293 – 333 Kio | **0** / 0 |
| Bibliothèque | **100** | 100 | 99 | 65 – 75 | 100 / 100 | 3,6 – 3,7 s | 420 – 850 ms | 239 Kio | **0** / 0 |
| Fiche | **100** | 100 | 97 | 62 – 72 | 100 / 100 | 3,7 – 4,2 s | 520 – 900 ms | 188 Kio | **0** / 0 |
| Recherche | **100** | 100 | 99 | 88 | 100 / 100 | 2,8 s | 280 ms | 114 Kio | **0** / 0 |
| Lecteur | 100 | 100 | 99 | 82 | 100 / 100 | 3,3 s | 280 ms | 324 Kio | 0 / 0 |
| Réglages | **100** | 100 | 96 | 74 – 79 | 100 / 100 | 3,1 – 3,4 s | 440 – 550 ms | 119 Kio | **0** / 0 |
| Profil | **100** | 100 | 99 | 68 – 81 | 100 / 100 | 3,3 – 4,0 s | 380 – 510 ms | 115 – 153 Kio | **0** / 0 |
| Connexion | 100 | 100 | 100 | 82 – 93 | **100 / 100** | 2,7 s | 130 ms | 100 Kio | 0 / 0 |

Quand deux passes successives donnent des valeurs différentes, la fourchette est indiquée ; en gras, ce qui change le verdict. En mobile, le LCP gagne 0,7 à 2 s et le poids transféré baisse de 35 à 65 %, grâce à la compression. Le TBT reste dominé par l'hydratation (voir plus bas).

**axe : 0 violation, tous niveaux confondus**, sur les 10 écrans en desktop et en mobile. **Accessibilité Lighthouse : 100 partout.** Les cibles de la release (accessibilité ≥ 95, aucune violation serious ou critical) sont atteintes.

### Poids de l'interface

| | Brut | gzip | brotli |
|---|---|---|---|
| JavaScript (109 fichiers, toutes routes) | 1 079 Kio | 398 Kio | 348 Kio |
| CSS (68 fichiers) | 215 Kio | 59 Kio | 50 Kio |

- Le plus gros morceau, 364 Kio brut et 97 Kio en brotli, est hls.js. Il n'est chargé que par le lecteur, ce qui explique le poids plus élevé de cette page.
- Le shell transféré passe de 401 à 148 Kio sur l'assistant, et de 253 à 100 Kio sur la page de connexion.
- **Polices** : Nunito variable, auto-hébergée, en `woff2` découpé par alphabet (`unicode-range`). Une page en français ne télécharge que le sous-ensemble latin (38 Kio), avec `font-display: swap`. Aucun appel à un service tiers.
- **Images** : variantes redimensionnées côté serveur (92 à 1 920 px) choisies par `srcset` et `sizes` ; JPEG ou PNG selon la source. Lighthouse suggère WebP ou AVIF (`modern-image-formats`) : Go n'a pas d'encodeur WebP en pur Go, le sujet est reporté (T-096).

### Reste à faire (performance mobile)

En mobile simulé, la performance reste entre 55 et 93 selon les pages (accueil : 46 avant, 55 à 62 après). Le TBT, de 130 ms à 1,6 s, est le principal poste : c'est l'hydratation de la SPA sur un processeur divisé par 4. Les pistes, regroupées dans T-097 :

- CSS global (`0.css`, 26 Kio) utilisé à 30 % sur une page donnée : le découper par route ;
- rendre les rangées hors écran de l'accueil de façon différée (`content-visibility` ou rendu à l'approche) ;
- mesurer sur un vrai téléphone de milieu de gamme avant d'aller plus loin.

## Latence de l'API sur 20 000 éléments

Le PLAN (M4) vise moins de 100 ms par page d'API pour 20 000 éléments.

`AETHER_LATENCY=1 go test ./internal/api -run Latency20k -v` crée une base synthétique de 20 500 éléments : 15 000 films et 500 séries de 10 épisodes, avec genres, distribution, classifications, notes, flux vidéo dont 10 % en 4K HDR, et index FTS. Chaque requête est jouée 25 fois en HTTP, en local, par un administrateur puis par un membre soumis au contrôle parental (−12 ans, non classés masqués). Le test échoue si un p95 dépasse 100 ms.

| Utilisateur | Requête | Avant p50 | Avant p95 | Après p50 | Après p95 |
|---|---|---|---|---|---|
| admin | parcours par titre (page 1, 60 éléments) | 37,0 | 60,4 | 42,6 | 60,2 |
| admin | parcours par date d'ajout | 46,6 | 60,5 | 49,7 | 58,4 |
| admin | parcours par note | 44,6 | 69,6 | 46,9 | 72,5 |
| admin | parcours aléatoire | 39,2 | 64,9 | 38,2 | 60,5 |
| admin | genre + années | 50,0 | 80,7 | 32,7 | 57,3 |
| admin | 4K + HDR | 126,5 | **187,4** | 77,2 | 95,4 |
| admin | non vus | 37,4 | 65,1 | 54,0 | 70,6 |
| admin | épisodes | 33,8 | 41,0 | 25,5 | 32,2 |
| admin | filtres de la bibliothèque | 165,7 | **210,4** | 1,0 | 1,7 |
| admin | accueil | 234,1 | **324,6** | 49,2 | 56,6 |
| admin | page 100 (curseur) | 258,7 | **349,2** | 67,6 | 100,0 |
| admin | recherche « ma » | 25,6 | 41,3 | 30,4 | 45,0 |
| admin | recherche « nuit claire » | 14,3 | 18,3 | 14,7 | 19,7 |
| membre −12 | parcours par titre | 152,1 | **238,3** | 35,3 | 40,6 |
| membre −12 | parcours par date d'ajout | 147,3 | **226,6** | 32,5 | 42,4 |
| membre −12 | parcours par note | 201,1 | **255,6** | 27,4 | 46,3 |
| membre −12 | parcours aléatoire | 175,6 | **248,6** | 26,8 | 39,9 |
| membre −12 | genre + années | 73,2 | **121,1** | 22,3 | 34,5 |
| membre −12 | 4K + HDR | 171,7 | **237,5** | 39,8 | 49,0 |
| membre −12 | non vus | 147,3 | **223,9** | 35,2 | 42,4 |
| membre −12 | épisodes | 84,8 | **123,6** | 29,9 | 46,9 |
| membre −12 | filtres | 183,7 | **246,6** | 0,7 | 2,6 |
| membre −12 | accueil | 761,2 | **1 047,6** | 40,0 | 47,8 |
| membre −12 | page 100 (curseur) | 207,6 | **298,6** | 36,3 | 52,0 |
| membre −12 | recherche « ma » | 32,2 | 47,5 | 34,5 | 43,6 |

Durées en millisecondes. En gras : au-dessus de l'objectif.

**Avant : 15 requêtes sur 28 dépassaient 100 ms au p95**, dont toutes celles d'un utilisateur sous contrôle parental, et l'accueil d'un membre atteignait 1 s. **Après : toutes les requêtes passent sous 100 ms au p95.** Le pire cas, la page 100 en administrateur, est à la limite.

Causes et correctifs :

1. **Colonnes calculées pour toutes les lignes.** Les sous-requêtes de chaque ligne (meilleur flux vidéo, nombre d'enfants, disponibilité de la source, données de l'utilisateur, épisodes non vus) étaient évaluées pour toutes les correspondances avant le tri et le `LIMIT`. Le parcours se fait désormais en deux temps : identifiants triés et total (`count(*) OVER ()`), puis colonnes complètes pour la seule page (`store.Browse`).
2. **Contrôle parental coûteux.** La condition `a.id IN (i.id, i.parent_id, (SELECT …))` reconstruisait une liste par ligne. Elle est réécrite en recherches directes par clé primaire sur le parent et le grand-parent : les listes d'un membre sont 4 à 6 fois plus rapides.
3. **Agrégats recalculés à chaque requête.** Les facettes de filtres et les genres de l'accueil parcouraient toute la bibliothèque. Ils sont mis en cache jusqu'à la prochaine transaction d'écriture validée, et 2 minutes au plus. Le premier calcul à froid prend environ 170 ms (T-099).
4. **Accueil séquentiel.** Ses sections indépendantes s'exécutent en parallèle (4 au plus, lectures concurrentes en WAL), sans calcul de total.
5. **Filtre 4K.** Il n'a pas besoin du maximum par élément : il cherche un flux d'au moins 3 200 × 1 800.

Point d'attention : la pagination reste fondée sur OFFSET. Son coût croît avec la profondeur (page 100 : 68 ms). Une pagination par clé (T-09A) serait à envisager au-delà de 50 000 éléments.

## Fiabilité de la base (T-089)

**Cause racine.** `Store.Tx` ouvrait des transactions `BEGIN DEFERRED`. Une transaction qui lit d'abord (le rafraîchissement relit l'élément, l'analyse cherche les éléments existants) puis écrit doit promouvoir son verrou. Si une autre connexion a validé entre-temps, SQLite refuse aussitôt : `SQLITE_BUSY_SNAPSHOT` (517) en WAL, ou `SQLITE_BUSY` (5) pour éviter un interblocage. Dans ces cas, `busy_timeout` ne s'applique pas. Le `busy_timeout` de 5 s était bien présent sur toutes les connexions ; il n'était donc pas en cause.

**Correctif.** Un pool d'écriture d'une seule connexion pour `Store.Tx`, avec `_txlock=immediate` (`BEGIN IMMEDIATE`) : les transactions s'enchaînent dans l'ordre au lieu d'échouer. Le pool principal sert les lectures et les écritures d'une seule instruction, avec un `busy_timeout` porté à 15 s.

**Vérification.**

- `store.TestConcurrentWritersNoLock` : 8 transactions concurrentes lecture-puis-écriture, 4 écrivains hors transaction et 4 lecteurs. Avant le correctif : `database is locked (517)` et `(5) (SQLITE_BUSY)` dès les premières itérations. Après : 0 erreur, aucune mise à jour perdue, aussi avec `-race`.
- De bout en bout, sur le vrai serveur : une bibliothèque de 3 000 films avec NFO (8 acteurs, 3 genres chacun) est analysée pendant 90 s de « Rafraîchir les métadonnées » et « Analyse complète » relancés chaque seconde, avec des lectures d'API en parallèle. L'ancien binaire produit 225 erreurs `database is locked` et autant de tâches réessayées. Le nouveau n'en produit aucune. Les deux finissent avec 3 000 films, 891 personnes et 27 000 crédits.

## Sécurité : garde SSRF (T-049, T-08A)

`internal/netguard` fournit le client HTTP du cache d'images. Il vérifie l'adresse **au moment de la connexion** (`net.Dialer.Control`, après résolution DNS), y compris pour chaque redirection. Il refuse :

- la boucle locale et les réseaux privés ;
- le lien local, dont `169.254.169.254` (métadonnées cloud) ;
- les ULA `fc00::/7`, dont `fd00:ec2::254` ;
- le multicast, le CGNAT (dont `100.100.100.200`) et les plages réservées ou de documentation ;
- les adresses IPv4 encapsulées dans de l'IPv6 (`::ffff:`, NAT64, 6to4).

Seuls http et https sont acceptés. Les délais sont bornés (10 s pour la connexion, 15 s jusqu'aux en-têtes, 30 s au total), les redirections limitées à 5 et la taille à 25 Mio. Avec un proxy sortant, la cible est vérifiée avant de lui être confiée.

Vérifié en réel : `image.tmdb.org`, ainsi que Cover Art Archive avec sa redirection vers `archive.org`, passent. `127.0.0.1` et `169.254.169.254` sont refusés.
