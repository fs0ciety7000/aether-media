# Site de présentation d'Aether

Site statique en Astro 5 qui présente Aether. Production : <https://aether.fs0ciety.org>.

```sh
npm ci
npm run dev       # http://localhost:4321
npm run build     # sortie statique dans dist/
```

- `src/pages/index.astro` : la page, ses textes et quelques lignes de JS vanilla (apparitions, écran incliné du hero, onglets, code d'appairage, bouton Copier).
- `src/styles/global.css` : tokens du thème Nuit (DESIGN_SYSTEM.md), mise en page et mouvement. `prefers-reduced-motion` est respecté.
- `src/assets/shots/` : captures réelles de l'application, recadrées depuis `docs/screenshots/` à la racine. Astro les sert en AVIF et WebP.
- `docs/SPEC.md` : spec de construction (tokens, typographie, structure, mouvement).
- `docs/screenshots/` : captures du site (desktop 1440 × 900, mobile 390 × 844).

## Docker

```sh
docker build -t aether-site .
docker run -p 8080:8080 aether-site
```

Build Node 22, puis service par `nginx-unprivileged` sur le port 8080 :
- cache immuable d'un an sur `/_astro/` ;
- CSP, HSTS, `X-Frame-Options`, `Referrer-Policy` et `Permissions-Policy` ;
- gzip ;
- healthcheck sur `/healthz`.

Le site est déployé par Coolify depuis ce dossier (`base_directory: /site`).
