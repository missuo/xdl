# syntax=docker/dockerfile:1
#
# XDL headless: the job queue, subscriptions, the HTTP API and MCP, without
# the GUI. Drive it with `xdl` from anywhere (XDL_API_URL + XDL_API_TOKEN),
# the browser extension (its options take the address and token), or an
# agent over MCP.
#
#   docker run -d --name xdl -p 47333:47333 \
#     -e XDL_AUTH_TOKEN=<x auth_token> -e XDL_API_TOKEN=<a secret> \
#     -v xdl-data:/data -v "$PWD/downloads:/downloads" ghcr.io/missuo/xdl

# The build runs on the builder's own architecture and cross-compiles (Go
# needs no cgo), so an arm64 image never compiles under emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/missuo/xdl/internal/version.Version=${VERSION}" \
      -o /out/xdl ./cmd/xdl

# xtractor is a glibc binary: Debian, not Alpine.
FROM debian:trixie-slim
ARG VERSION=dev
LABEL org.opencontainers.image.title="XDL" \
      org.opencontainers.image.description="Batch downloader for X/Twitter media: job queue, subscriptions, API and MCP" \
      org.opencontainers.image.source="https://github.com/missuo/xdl" \
      org.opencontainers.image.licenses="GPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}"
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates ffmpeg libimage-exiftool-perl \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --create-home --uid 1000 xdl \
 && mkdir -p /data /downloads \
 && chown xdl:xdl /data /downloads
COPY --from=build /out/xdl /usr/local/bin/xdl

# /data holds the library, the download history, subscriptions, settings
# and xtractor (installed on first start); /downloads the media.
ENV HOME=/data \
    XDG_CONFIG_HOME=/data/config \
    XDL_DOWNLOAD_DIR=/downloads \
    XDL_API_ADDR=0.0.0.0:47333
USER xdl
VOLUME ["/data", "/downloads"]
EXPOSE 47333
HEALTHCHECK --interval=1m --timeout=5s CMD ["xdl", "status"]
ENTRYPOINT ["xdl"]
CMD ["serve", "-install-deps"]
