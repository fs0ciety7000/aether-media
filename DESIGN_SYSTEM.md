# Aether — Design System (v0.3)

> **Intention** : un tableau de bord de streaming moderne, bleu nuit, net et généreux. Les visuels sont la star ; l'interface les encadre par des panneaux calmes.
> **Référence validée par le PO** : le projet Behance « Nexify – Movie Streaming Platform UI Design ». On reprend sa structure et son ambiance, avec un accent **teal adouci** (pas de cyan néon).
> v0.1–0.2 (« salle de cinéma privée », serif + laiton) sont abandonnées comme direction par défaut. Leurs palettes survivent sous forme de thèmes alternatifs.

---

## 1. Principes

1. **Le contenu d'abord.** Les affiches, fonds et vignettes apportent la couleur ; l'UI est bleu nuit, neutre et mate.
2. **Tableau de bord, pas vitrine.** L'accueil est une grille dense mais aérée : hero en carte, catégories, mosaïque « Reprendre », rangées.
3. **Panneaux et tuiles.** Tout est posé sur des surfaces arrondies, avec une légère teinte teal au coin supérieur gauche (`--tint`). Pas de bordures dures.
4. **Un seul accent, adouci.** Le teal sert à l'action principale, à la progression, à l'état actif et au focus. Jamais de lueur ni de bordure lumineuse.
5. **Une seule famille typographique.** Nunito, arrondie et lisible, partout.
6. **Mouvement court et fluide.** Fondus et légères translations, aucun rebond.
7. **Accessible par défaut.** Contraste AA vérifié en CI, focus visible, navigation clavier.

### À proscrire
Cyan ou magenta saturés · glow et bordures lumineuses · ombres colorées · noir pur `#000` en fond d'interface · emojis dans l'UI · badges multicolores.

---

## 2. Couleurs — tokens

Les tokens sont **sémantiques** ; un thème ne redéfinit que ces variables. Source de vérité : `web/src/lib/styles/tokens.css`.

### Thème par défaut — « Nuit »
| Token | Valeur | Rôle |
|---|---|---|
| `--bg` | `#04121D` | fond de l'application |
| `--bg-sunken` | `#030D16` | zones en retrait, champs |
| `--surface` | `#071725` | panneaux, tuiles de navigation |
| `--surface-raised` | `#0A1F2F` | menus, popovers |
| `--surface-active` | `#0D2F3D` | élément actif (tuile de nav, sélection) |
| `--line` / `--line-strong` | `rgba(246,255,255,.06)` / `.12` | séparateurs |
| `--text` | `#F6FFFF` | texte principal |
| `--text-muted` | `#8A9FA7` | secondaire (la référence `#778E97` échouait l'AA sur `surface-active`) |
| `--text-faint` | `#5B7480` | légendes, placeholders |
| `--accent` | `#5EC4C0` | teal adouci : action, progression, focus |
| `--accent-ink` | `#6FCDC9` | accent pour petit texte |
| `--accent-contrast` | `#04121D` | texte sur l'accent (9:1) |
| `--accent-soft` | `rgba(94,196,192,.13)` | fonds teintés |
| `--danger` / `--success` | `#E48A7A` / `#7CC79F` | états |
| `--on-image` | `#F6FFFF` | texte posé sur une image |
| `--tint` | radial teal 15 % → 0 au coin haut-gauche | matière des panneaux |

### Thèmes alternatifs
| Thème | Ambiance | Accent | Texte sur accent |
|---|---|---|---|
| **Nuit** (défaut) | bleu nuit, teal | `#5EC4C0` | `#04121D` |
| Salle obscure | charbon chaud | `#C9A673` ambre | `#1A1510` |
| Velours | bordeaux sombre | `#CF8E80` rose cuivré | `#1A0F0E` |
| Ardoise | gris minéral | `#93B6C7` bleu acier | `#0F1417` |
| Noyer | bois sombre | `#C49C64` noyer | `#17120B` |
| Lin (clair) | papier chaud | `#2C7B77` teal profond | `#FFFFFF` |

Mode **Système** : Nuit ↔ Lin selon `prefers-color-scheme`. Chaque thème définit aussi `surface-active` et `tint` dans son accent. Contrastes vérifiés par `npm run check:contrast` (CI).

---

## 3. Typographie

**Nunito** (variable, OFL), auto-hébergée dans le binaire. Aucun appel à Google Fonts.

| Token | Taille / interligne | Graisse | Usage |
|---|---|---|---|
| `--fs-hero` | 30 → 40 px / 112 % | 800 | titre du hero |
| `--fs-h1` | 40 px / 120 % | 700 | titre de page |
| `--fs-h2` | 20 px / 135 % | 700 | en-têtes de section (« Reprendre ») |
| `--fs-body` | 14 px / 150 % | 400 | texte courant, synopsis |
| `--fs-small` | 13 px | 500 | métadonnées, durées |
| `--fs-micro` | 12 px | 600 | tags, badges |

Durées et temps : `font-variant-numeric: tabular-nums`.

---

## 4. Espacement, formes

- **Espacement (base 4)** : `--sp-1` 4 px → `--sp-10` 128 px. `--gap` entre blocs de la grille.
- **Gouttières** : 16 px (mobile) · 24–32 px (desktop).
- **Rayons** : `xs` 6 · `sm` 10 (tuiles de nav, catégories) · `md` 12 (vignettes) · `lg` 16 (panneaux) · `xl` 20 (hero) · `full` (boutons pilules, avatars).
- **Ratios** : poster 2:3 · vignette 16:9 · catégorie ~16:7.

## 5. Élévation et matière
- Ombres neutres, larges et diffuses (`--shadow-1..3`).
- Panneaux : `--surface` + `--tint`.
- Grain pellicule : optionnel, désactivé par défaut.

## 6. Mouvement
`--dur-fast` 140 ms · `--dur-base` 220 ms · `--dur-slow` 420 ms, easing de sortie douce. Images en fondu depuis leur couleur dominante. `prefers-reduced-motion` : opacité seule.

## 7. Iconographie
Trait 1.5 px, style Lucide, `currentColor`, dessinées à la main dans `Icon.svelte`. Les boutons-icônes sont des **tuiles carrées arrondies** (`IconButton`).

---

## 8. Structure et composants

### App shell (desktop)
- **Sidebar** (248 px, repliable en rail de 76 px, état mémorisé) : logo + « Aether », puis chaque entrée est une **tuile** arrondie. L'entrée active passe en `surface-active` avec l'icône en accent.
  - Groupes : Accueil, Explorer, Favoris, Genres · Ma collection, Récemment ajoutés, Reprendre, Collections.
  - Réglages épinglés en bas ; pied : lien système de design, version serveur, bouton replier.
- **Top bar** (sticky, opaque) : recherche large à gauche (`/` pour focus) ; à droite, sélecteur de thème, notifications (tuile + pastille) et bloc utilisateur (avatar, nom, e-mail).

### Mobile (< 768 px)
Barre d'onglets en bas : Accueil, Explorer, Recherche, Favoris, Profil. Le hero passe en pleine largeur ; les catégories défilent horizontalement ; la mosaïque passe sur 2 colonnes, avec la tuile vedette en premier.

### Accueil
1. **Hero en carte** (~2/3) : tags en haut à gauche, badge « Tendance » en haut à droite, titre sur 2 lignes, métadonnées, synopsis. Boutons « Lecture » (pilule accent) et « Plus d'infos » (pilule sombre). Rotation 8 s, pausable.
2. **Panneau Catégories** (~1/3) : grille 2×4 de tuiles (image + libellé centré) et un lien « Tout voir ».
3. **Reprendre** : mosaïque avec 2 vignettes moyennes au-dessus de 3 petites, et une tuile vedette sur la hauteur des deux rangées. Chaque tuile affiche le temps écoulé, la durée totale et une fine barre teal.
4. **Vous aimerez aussi** et **Récemment ajoutés** : rangées horizontales.

### Inventaire
`Logo`, `Button` (pilule : primaire, secondaire, tertiaire), `IconButton`, `Avatar`, `UserChip`, `Tag`, `Chip`, `Input`, `SearchField`, `SectionHeader`, `Hero`, `CategoryTile`, `CategoriesPanel`, `ContinueTile`, `ContinueBento`, `PosterCard`, `ThumbCard`, `Row`, `ProgressLine`, `Skeleton`, `Toaster`, `ThemeSwitcher`, `Stepper` (parcours guidés), `FolderPicker` (explorateur de dossiers : fil d'Ariane, dossiers seulement), ainsi que le shell (`Sidebar`, `AppHeader`, `TabBar`).

## 9. Écrans v1
1. Connexion / assistant de premier lancement
2. Accueil (ci-dessus)
3. Bibliothèque : grille + filtres + tri
4. Détail film / série / saison / album
5. Lecteur vidéo, mini-lecteur audio
6. Recherche
7. Collections, favoris
8. Profil et préférences
9. Administration

## 10. Implémentation
- Tokens : `web/src/lib/styles/tokens.css` ; thèmes via `[data-theme]` sur `<html>`, appliqués avant le premier rendu pour éviter le flash.
- Galerie vivante : route `/design`. Captures de référence : `docs/screenshots/m1b/`.
- Les images de la référence Behance ne sont **jamais** copiées dans le dépôt.
