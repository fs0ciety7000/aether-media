# Aether

Serveur multimédia self-hosted, open source, sobre et premium. Films, séries, musique, photos : une salle de cinéma privée dans le navigateur.

**Installation** (Docker, .deb / .rpm, Homebrew, Windows, agent sur une autre machine) : voir [docs/INSTALL.md](docs/INSTALL.md).

```sh
docker run -d -p 8096:8096 -v ./data:/data -v /srv/media:/media:ro ghcr.io/fs0ciety7000/aether
```

> Projet en phase de cadrage. Voir :
> - [PLAN.md](PLAN.md) : vision, périmètre, jalons
> - [ARCHITECTURE.md](ARCHITECTURE.md) : stack proposée, modules, modèle de données
> - [DESIGN_SYSTEM.md](DESIGN_SYSTEM.md) : direction artistique et tokens
> - [TASKS.md](TASKS.md) : suivi des tâches

## Licence

Aether est distribué sous licence [GNU AGPL-3.0](LICENSE) (`AGPL-3.0-only`). Si vous hébergez une version modifiée accessible à d'autres, vous devez en publier le code source.
