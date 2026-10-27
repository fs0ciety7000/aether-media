# Aether — Design System (draft v0.1)

> **Intention** : une salle de cinéma privée. Velours, bois sombre, lumière tamisée, silence avant la séance.
> **Luxe discret** : mat, ombres douces, beaucoup d'espace. L'affiche est la star ; l'interface s'efface.

---

## 1. Principes

1. **Le contenu d'abord.** Les affiches, fonds et photos apportent la couleur. L'UI reste neutre, chaude, mate.
2. **Retenue.** Aucun néon, aucun dégradé saturé, aucune lueur (glow), aucun glassmorphism criard. Une seule couleur d'accent, désaturée, utilisée avec parcimonie.
3. **Espace généreux.** Grandes marges, rythme vertical lent. On n'entasse pas : on ne montre que ce qui sert.
4. **Profondeur douce.** Hiérarchie portée par la valeur (clair/foncé) et des ombres diffuses, pas par des bordures dures.
5. **Mouvement feutré.** Transitions lentes et courtes (fondus, légères translations). Jamais de rebond ni d'effet élastique.
6. **Typographie éditoriale.** Un serif élégant pour les titres (l'affiche, le générique), un sans-serif sobre pour l'interface.
7. **Accessible par défaut.** Contraste AA minimum, focus visible (et beau), navigation clavier et télécommande (10-foot UI plus tard).

### À proscrire
Néons, cyan, magenta, violets électriques · bordures lumineuses · ombres colorées · noir pur `#000` en fond d'interface · blanc pur `#fff` en texte courant · emojis dans l'UI · badges multicolores · animations « fun ».

---

## 2. Couleurs — tokens

Les tokens sont **sémantiques**. Un thème ne redéfinit que ces variables.

| Token | Rôle |
|---|---|
| `--bg` | fond de l'application |
| `--bg-sunken` | zones en retrait (lecteur, barres) |
| `--surface` | cartes, panneaux |
| `--surface-raised` | menus, modales, popovers |
| `--line` | séparateurs très discrets |
| `--text` | texte principal |
| `--text-muted` | texte secondaire, métadonnées |
| `--text-faint` | légendes, placeholders |
| `--accent` | action principale, progression, focus |
| `--accent-contrast` | texte sur l'accent |
| `--danger` / `--success` | états (désaturés) |
| `--scrim` | voile sur les images (lisibilité du texte) |

### Thème par défaut — « Salle obscure » (sombre, chaud)
```css
--bg:             #121010;  /* charbon chaud, pas noir pur */
--bg-sunken:      #0C0B0A;
--surface:        #1A1716;
--surface-raised: #221E1C;
--line:           rgba(237, 230, 218, 0.07);
--text:           #EDE6DA;  /* ivoire */
--text-muted:     #A89F92;
--text-faint:     #6E665C;
--accent:         #B89A6A;  /* laiton patiné */
--accent-contrast:#1A1510;
--danger:         #B4675A;  /* terre cuite */
--success:        #7F9474;  /* sauge */
--scrim:          linear-gradient(to top, rgba(18,16,16,.92) 0%, rgba(18,16,16,.55) 40%, rgba(18,16,16,0) 75%);
```

### Thèmes additionnels (sobres)
| Thème | Ambiance | `--bg` | `--surface` | `--text` | `--accent` |
|---|---|---|---|---|---|
| **Velours** | rideau bordeaux, très sombre | `#140F10` | `#1D1617` | `#EFE5DF` | `#A8786A` (vieux rose cuivré) |
| **Ardoise** | froid, minéral, nordique | `#101214` | `#181B1E` | `#E4E6E8` | `#8FA3AE` (bleu-gris) |
| **Noyer** | bois sombre, bibliothèque | `#13110E` | `#1C1915` | `#ECE4D4` | `#A8875A` (ambre éteint) |
| **Lin** (clair) | matinée, papier, galerie | `#F3EFE8` | `#FBF9F5` | `#2A2622` | `#8A6E45` (bronze) |

Option utilisateur : **« Teinte ambiante »**. L'arrière-plan de la page de détail reprend la couleur dominante de l'affiche, **désaturée à ≤ 20 %** et assombrie : une ambiance, jamais un aplat vif.

---

## 3. Typographie

Les polices sont **auto-hébergées** dans le binaire (aucun appel à Google Fonts : self-hosted et vie privée). Toutes sous licence OFL.

| Usage | Police | Notes |
|---|---|---|
| Display (titres de films, en-têtes de section) | **Cormorant Garamond** (500/600) | serif de générique, élégant ; tracking légèrement négatif sur les grandes tailles |
| UI et texte courant | **Instrument Sans** (400/500/600) | sans-serif sobre et lisible ; chiffres tabulaires pour les durées |
| Technique (codecs, débits) | Instrument Sans + `font-variant-numeric: tabular-nums` | pas de monospace visible |

### Échelle (ratio ~1.25, base 16 px)
| Token | Taille / interligne | Usage |
|---|---|---|
| `--fs-hero` | clamp(2.75rem, 5vw, 4.5rem) / 1.05 | titre du hero, page de détail |
| `--fs-h1` | 2.25rem / 1.15 | titre de page |
| `--fs-h2` | 1.5rem / 1.25 | rangées (« Reprendre », « Récemment ajoutés ») |
| `--fs-h3` | 1.125rem / 1.35 | titres de carte |
| `--fs-body` | 1rem / 1.6 | synopsis, texte |
| `--fs-small` | 0.875rem / 1.5 | métadonnées |
| `--fs-micro` | 0.75rem / 1.4, +0.06em, MAJUSCULES | sur-titres (« FILM · 2019 · 2 h 12 ») |

Règles : 60–72 caractères maximum par ligne pour les synopsis. Pas de gras dans le serif au-delà de 600. Italique serif autorisé pour les citations et taglines.

---

## 4. Espacement, grille, formes

- **Espacement (base 4)** : `--sp-1: 4px` · `2: 8` · `3: 12` · `4: 16` · `5: 24` · `6: 32` · `7: 48` · `8: 64` · `9: 96` · `10: 128`.
- **Gouttières de page** : 16 px (mobile), 32 px (tablette), 56–80 px (desktop).
- **Rangées** : 48–64 px entre deux rangées de la page d'accueil.
- **Rayons** : `--r-sm: 4px` (chips) · `--r-md: 8px` (cartes, affiches) · `--r-lg: 14px` (modales) · `--r-full` (avatars, boutons ronds du lecteur). Pas de formes « pilule » partout.
- **Ratios d'affiches** : poster 2:3 · backdrop / thumb 16:9 · album 1:1 · logo libre.

## 5. Élévation et matière
```css
--shadow-1: 0 1px 2px rgba(0,0,0,.25), 0 2px 8px rgba(0,0,0,.18);
--shadow-2: 0 4px 12px rgba(0,0,0,.28), 0 12px 32px rgba(0,0,0,.22);
--shadow-3: 0 12px 24px rgba(0,0,0,.32), 0 32px 64px rgba(0,0,0,.30);
```
- Les ombres sont toujours **neutres** (jamais colorées), larges et diffuses.
- **Grain** optionnel : bruit SVG à 2–3 % d'opacité sur `--bg`, pour une sensation pellicule et mate (désactivable).
- Les images reçoivent un `--scrim` sous le texte, jamais un flou criard.

## 6. Mouvement
| Token | Valeur | Usage |
|---|---|---|
| `--ease` | `cubic-bezier(.2, .7, .2, 1)` | standard (sortie douce) |
| `--dur-fast` | 150 ms | survols, focus |
| `--dur-base` | 260 ms | apparitions, menus |
| `--dur-slow` | 480 ms | changement de hero, fondus d'images |

- Les images apparaissent en fondu depuis leur **blurhash / couleur dominante** (jamais de saut).
- Survol d'une carte : élévation `shadow-1 → shadow-2` et `scale(1.02)` au maximum.
- `prefers-reduced-motion` : on garde uniquement les fondus d'opacité.

## 7. Iconographie
- Trait fin (1.5 px), coins arrondis, style **Lucide** (ISC) ou set maison équivalent. Taille 20/24 px.
- Monochromes (`currentColor`). Pas d'icônes remplies sauf pour l'état actif (favori, lecture).

---

## 8. Composants (inventaire v1)

| Composant | Notes de design |
|---|---|
| **App shell** | barre latérale fine et rétractable (desktop) ou barre basse (mobile) ; en-tête transparent qui s'opacifie au scroll |
| **Hero** | backdrop plein cadre, scrim bas, logo ou titre serif, 1 action principale (Lecture) + 1 secondaire ; rotation lente (8 s, fondu) |
| **Rangée (Row)** | titre H2 serif + lien « Tout voir » discret ; défilement horizontal avec flèches au survol ; snap |
| **Carte poster** | 2:3, rayon `r-md`, `shadow-1` ; sous l'affiche : titre et année en `text-muted` ; barre de progression fine (2 px, accent) |
| **Carte épisode / thumb** | 16:9, numéro + titre, durée restante |
| **Page de détail** | backdrop atténué en fond, colonne poster + bloc infos ; métadonnées en micro-capitales séparées par « · » ; casting en rangée d'avatars ronds désaturés |
| **Boutons** | Primaire : fond accent, texte `accent-contrast`. Secondaire : surface + texte. Tertiaire : texte seul. Hauteur 40/48 px |
| **Champs** | fond `bg-sunken`, aucune bordure visible au repos ; ligne accent au focus |
| **Chips / filtres** | `r-sm`, surface, sélection = texte accent + fond légèrement teinté |
| **Lecteur** | plein écran noir chaud ; contrôles qui apparaissent en fondu, barre de progression fine qui s'épaissit au survol ; panneau latéral pour audio et sous-titres ; badge discret « Direct » / « Transcodage » |
| **Toasts** | coin bas, surface-raised, sans icône colorée |
| **Squelettes** | aplats `surface` avec une respiration d'opacité très lente (pas de shimmer brillant) |
| **Focus** | anneau 2 px `accent` décalé de 2 px, partout |

## 9. Écrans v1
1. Connexion / premier lancement (assistant : admin → bibliothèques → métadonnées)
2. Accueil : hero, Reprendre, À suivre, Récemment ajoutés (par bibliothèque), Collections
3. Bibliothèque : grille + filtres (genre, année, note, vu / non vu, résolution) + tri
4. Détail film / série / saison / album
5. Lecteur vidéo, mini-lecteur audio
6. Recherche (instantanée, résultats groupés par type)
7. Collections
8. Profil et préférences (thème, langue audio et sous-titres)
9. Administration : bibliothèques, utilisateurs, tâches, transcodage, journaux

## 10. Implémentation
- Tokens dans `web/src/lib/styles/tokens.css`, et thèmes via `[data-theme="velours"]` sur `<html>`.
- Thème `system` : suit `prefers-color-scheme` (Salle obscure ↔ Lin).
- Contrastes vérifiés automatiquement en CI (texte/fond ≥ 4.5:1, texte large ≥ 3:1).
- **Note de méthode** : le skill `frontend-design` n'est pas installé dans cette session. Les principes ci-dessus en reprennent l'esprit (direction affirmée, typographie distinctive, pas d'esthétique « IA générique »). Il sera utilisé pour les maquettes dès qu'il sera disponible.
