# Site Aether : spec de construction

## Brief
- **Quoi / pour qui** : page de présentation d'Aether, serveur multimédia self-hosted (Go + SvelteKit, AGPL-3.0). Visiteur : amateur de homelab / NAS / seedbox qui connaît Plex, Emby ou Jellyfin et cherche quelque chose de plus beau, plus calme, et libre.
- **Humeur** : nocturne, calme, précis.
- **Signature** : « l'écran s'allume ». Sous le titre, la vraie capture de l'accueil d'Aether est posée dans un cadre incliné en perspective (rotateX 22°, échelle 0,9, luminosité 0,55). Au défilement, elle se redresse et s'éclaire comme un écran de salle qui démarre. Un faisceau de projecteur très doux (gradient linéaire à 6 % d'opacité, sans glow) la surplombe.
- **Second moment** : le code d'appairage de l'agent (`K7PQ-4XRM`) se compose caractère par caractère, comme dans l'application.
- **Sortie** : Astro 5 statique, CSS natif, ~3 Ko de JS vanilla, sans bibliothèque d'animation. Images optimisées par `astro:assets` (AVIF + WebP, `srcset`).

## Tokens (DESIGN_SYSTEM.md, thème Nuit)
| Token | Valeur |
|---|---|
| `--bg` | `#04121D` (premier `html, body`) |
| `--bg-sunken` | `#030D16` |
| `--surface` / `--surface-raised` | `#071725` / `#0A1F2F` |
| `--line` / `--line-strong` | `rgba(246,255,255,.07)` / `.12` |
| `--text` | `#F6FFFF` |
| `--text-muted` | `#8A9FA7` (≥ 6,5:1 sur `--bg`) |
| `--accent` | `#5EC4C0` ; texte sur l'accent `#04121D` |
| `--accent-soft` | `rgba(94,196,192,.13)` |
| Sélection | `rgba(94,196,192,.3)` |

Rayons : 10 / 16 / 20 / 28 px, pilules `999px`. Ombres neutres : `0 30px 80px -20px rgba(0,0,0,.6)`. Aucune ombre colorée, aucun glow.

## Typographie
Une seule famille, **Nunito Variable**, auto-hébergée (`@fontsource-variable/nunito`), `font-display: swap`.
| Rôle | Taille | Interligne | Graisse | Approche |
|---|---|---|---|---|
| H1 | `clamp(2.6rem, 7.4vw, 6.4rem)` | 0,98 | 850 | -0,045em ; ligne 2 en `--text-muted` |
| H2 de chapitre | `clamp(2.1rem, 4.6vw, 4rem)` | 1,02 | 820 | -0,035em ; seconde ligne atténuée |
| Lede | `clamp(1.05rem, 1.4vw, 1.25rem)` | 1,55 | 450 | max 38rem |
| Corps | 16 px | 1,6 | 450 | |
| Étiquettes | 12 px, majuscules | 1,2 | 750 | +0,2em ; numéros `01 —` |

## Structure et textes
1. **Nav** flottante en pilule (verre sombre `rgba(7,23,37,.72)` + flou 14 px) : logo A + « Aether », liens Bibliothèque · Lecture · Sources · Foyer, puis GitHub (icône) et le CTA « Installer ». Sous 860 px : logo + CTA seulement.
2. **Hero** (centré) : pastille « Libre · AGPL-3.0 », puis le H1 « Vos films méritent / une salle privée. » Lede : « Aether range vos films, séries et musique où qu'ils dorment (disque local, NAS, seedbox, autre machine) et les rejoue partout, en qualité d'origine dès que l'appareil le permet. » CTA : « Installer Aether » et « Code source ». Dessous, l'écran signature (capture `accueil`).
3. **Bande** de quatre faits : 1 binaire (serveur et agent) · 3 types de sources · 6 thèmes · 0 compte cloud (aucune télémétrie par défaut).
4. **01 Bibliothèque** : « Pointez un dossier. / Retrouvez une cinémathèque. » Points : scan et surveillance, noms *arr, TMDB, NFO prioritaire, MusicBrainz, champs verrouillés, « À identifier ». Capture `film`.
5. **02 Lecture** : « La meilleure image / que l'écran accepte. » Sélecteur segmenté Direct / Remux / Transcodage qui change le texte et le panneau « Informations de lecture » reconstitué en HTML. Capture `lecteur`.
6. **03 Sources** : « Vos fichiers restent / où ils sont. » Trois étapes de l'agent (découverte, code, dossiers), code animé, SFTP, plusieurs sources par bibliothèque. Captures `agent` et `sources`, plus le téléphone `mobile-code`.
7. **04 Foyer** : « Un serveur. / Chacun sa séance. » Bento : utilisateurs et contrôle parental (`utilisateur`), Reprendre et À suivre (`reprendre`), collections (`collections`), PWA (téléphone `mobile-accueil`).
8. **05 Synchronisation** : « Ce que vous regardez ici / compte aussi là-bas. » Trakt et Simkl cités en texte seulement. Capture `synchro`.
9. **Installer** : onglets Docker · Debian / Ubuntu · Fedora / RHEL · Homebrew · Windows, avec des blocs de code copiables tirés de docs/INSTALL.md.
10. **Fin** : « La salle est prête. / Il ne manque que vos films. » Pied de page : licence, liens, mention des marques tierces.

## Mouvement
- Entrée du hero : nav (0 s), pastille (0,1 s), lignes du H1 montant de 110 % dans un masque (0,18 s / 0,3 s, 0,9 s, `cubic-bezier(.16,1,.3,1)`), lede (0,45 s), CTA (0,55 s), écran (0,6 s, fondu 1,2 s).
- Écran signature : variables pilotées par le défilement sur les 70 premiers vh ; lerp 0,14, et la boucle rAF ne tourne que pendant que la valeur se stabilise.
- Apparitions au défilement : IntersectionObserver au seuil 0,15, y 28 px, 0,8 s, décalage de 90 ms.
- Survols : 200 ms ; `:active` à l'échelle 0,97.
- `prefers-reduced-motion` : écran à plat, aucune translation, opacité seule ; le code d'appairage est affiché d'emblée.
- Sans JS : tout reste visible (la classe `js` sur `<html>` active seule les états initiaux).

## Accessibilité et performance
- Lien d'évitement, `:focus-visible` (anneau de 2 px en accent, décalé de 3 px), onglets ARIA (`tablist`, flèches du clavier), `aria-live` sur le panneau de lecture.
- Images non critiques en lazy ; la capture du hero en `eager` et `fetchpriority=high` ; dimensions toujours réservées.
- Cibles tactiles ≥ 44 px, aucun défilement horizontal (`overflow-x: clip`).
