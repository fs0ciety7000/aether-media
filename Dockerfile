# syntax=docker/dockerfile:1

FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-bookworm AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/fs0ciety7000/aether-media/internal/version.Version=${VERSION} -X github.com/fs0ciety7000/aether-media/internal/version.Commit=${COMMIT}" \
      -o /out/aether ./cmd/aether

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --uid 1000 --home /data aether \
 && mkdir -p /data /media && chown aether /data
COPY --from=server /out/aether /usr/local/bin/aether
USER aether
ENV AETHER_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8096
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["aether", "healthcheck"]
ENTRYPOINT ["aether"]
CMD ["serve"]
