# syntax=docker/dockerfile:1

# Image publiée par GoReleaser (dockers_v2) : reprend l'étape finale du
# Dockerfile racine, mais copie le binaire déjà compilé par GoReleaser
# (contexte : linux/amd64/aether, linux/arm64/aether).
# À garder synchronisé avec l'étape finale de ../../Dockerfile.

FROM debian:trixie-slim
# Clé du dépôt Jellyfin (empreinte vérifiée au build).
ADD --chmod=644 --checksum=sha256:a0cde241ae297fa6f0265c0bf15ce9eb9ee97c008904a59ab367a67d59532839 \
    https://repo.jellyfin.org/jellyfin_team.gpg.key /etc/apt/keyrings/jellyfin.asc
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata \
 && printf 'Types: deb\nURIs: https://repo.jellyfin.org/debian\nSuites: trixie\nComponents: main\nSigned-By: /etc/apt/keyrings/jellyfin.asc\n' \
      > /etc/apt/sources.list.d/jellyfin.sources \
 && apt-get update \
 && apt-get install -y --no-install-recommends jellyfin-ffmpeg7 \
 && ln -s /usr/lib/jellyfin-ffmpeg/ffmpeg /usr/local/bin/ffmpeg \
 && ln -s /usr/lib/jellyfin-ffmpeg/ffprobe /usr/local/bin/ffprobe \
 && rm -rf /var/lib/apt/lists/* /var/cache/apt/* /var/log/apt /var/log/dpkg.log \
           /usr/share/doc/* /usr/share/man/* \
 && useradd --system --uid 1000 --home /data aether \
 && mkdir -p /data /media && chown aether /data
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/aether /usr/local/bin/aether
USER aether
ENV AETHER_DATA_DIR=/data \
    AETHER_HOST=0.0.0.0 \
    NVIDIA_DRIVER_CAPABILITIES=compute,video,utility
VOLUME ["/data"]
EXPOSE 8096
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["aether", "healthcheck"]
ENTRYPOINT ["aether"]
CMD ["serve"]
