# Aether — interface web

SPA SvelteKit (Svelte 5, TypeScript, `adapter-static`). Le build est écrit dans `../internal/webui/dist`
et embarqué dans le binaire Go. Direction artistique : `../DESIGN_SYSTEM.md` ; contrat d’API : `../docs/API.md`.

```bash
npm ci
npm run dev             # Vite sur :5173, proxy /api → $AETHER_API (défaut http://127.0.0.1:8096)
npm run check           # svelte-check (0 erreur, 0 avertissement attendus)
npm run check:contrast  # contrastes AA de tous les thèmes
npm run build           # → ../internal/webui/dist (puis `make build` à la racine pour l’embarquer)
```

## Organisation

| Dossier | Contenu |
|---|---|
| `src/lib/api/` | client typé (`client.ts`, erreurs `ApiError` code/message), types (`types.ts`), URL d’images (`images.ts` : `imageUrl(img, largeur)`, `imageSrcset`), décodeur blurhash (`blurhash.ts`), flux SSE partagé (`events.svelte.ts`) et magasins (`stores.svelte.ts` : bibliothèques, infos serveur, analyses en cours, éléments à identifier) |
| `src/lib/components/` | composants du système de design ; `browse/` (grille de bibliothèque), `detail/` (fiche), `items/` (modifier, identifier), `settings/`, `home/`, `shell/` |
| `src/routes/` | `/` accueil · `/bibliotheque/[id]` · `/parcourir?genre=…|personne=…` · `/item/[id]` · `/recherche?q=` · `/a-identifier` · `/reglages/bibliotheques` · `/design` (galerie vivante) |

Les filtres de la grille vivent dans l’URL (`sort`, `order`, `genre`, `de`, `a`, `res`, `hdr`, `vue=albums`),
ce qui rend chaque vue partageable et compatible avec Précédent/Suivant.

## Bibliothèque de démonstration

`scripts/demo-library.sh` crée une arborescence réaliste avec ffmpeg : 12 films (NFO complets, affiches et
fonds générés), 2 séries (2 saisons × 3 épisodes, NFO, vignettes) et 3 albums de 2 artistes (FLAC étiquetés,
pochettes). Les titres sont composés avec `drawtext` si une police est trouvée (`fc-match`), sinon dégradés seuls.
Un `aether.toml` de démo (MusicBrainz désactivé, donc aucun appel réseau) est écrit à côté.

```bash
# 1. Médias et serveur
web/scripts/demo-library.sh /tmp/aether-demo/media
make build                                   # à la racine du dépôt → ./bin/aether

# 2. Bibliothèques (dans un dossier de données dédié)
export AETHER_DATA_DIR=/tmp/aether-demo/data
mkdir -p "$AETHER_DATA_DIR" && cp /tmp/aether-demo/media/aether.toml "$AETHER_DATA_DIR/"
./bin/aether library add -name Films   -kind movies -path /tmp/aether-demo/media/Films
./bin/aether library add -name Séries  -kind shows  -path /tmp/aether-demo/media/Series
./bin/aether library add -name Musique -kind music  -path /tmp/aether-demo/media/Musique
./bin/aether scan                            # analyse + ffprobe + métadonnées locales

# 3. Servir
AETHER_PORT=8096 ./bin/aether serve          # UI embarquée sur http://localhost:8096

# 4. (optionnel) développer l’UI contre ce serveur
cd web && AETHER_API=http://127.0.0.1:8096 npm run dev
```

Sans clé TMDB (`metadata.tmdb_api_key` ou `AETHER_TMDB_API_KEY`), films et séries ne sont décrits que par les NFO
et images locales ; la page « À identifier » l’explique et la recherche de correspondances renvoie 503.
