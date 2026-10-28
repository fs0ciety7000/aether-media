# syntax=docker/dockerfile:1

# Image Aether : binaire Go statique + jellyfin-ffmpeg (FFmpeg 7 avec VAAPI,
# QSV, NVENC, zscale/tonemap, libass). Multi-arch : linux/amd64, linux/arm64.
#   docker buildx build --platform linux/amd64,linux/arm64 -t aether .

# Les étapes de compilation tournent sur l'architecture de la machine de build
# (pas d'émulation) : le web est indépendant de l'architecture et Go compile
# en croisé sans CGO.
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
ARG VERSION=dev
ARG COMMIT=unknown
ARG TARGETOS=linux
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/fs0ciety7000/aether-media/internal/version.Version=${VERSION} -X github.com/fs0ciety7000/aether-media/internal/version.Commit=${COMMIT}" \
      -o /out/aether ./cmd/aether

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
COPY --from=server /out/aether /usr/local/bin/aether
USER aether
ENV AETHER_DATA_DIR=/data \
    AETHER_HOST=0.0.0.0 \
    NVIDIA_DRIVER_CAPABILITIES=compute,video,utility
VOLUME ["/data"]
EXPOSE 8096
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["aether", "healthcheck"]
ENTRYPOINT ["aether"]
CMD ["serve"]
