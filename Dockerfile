# XDL headless: the job queue, subscriptions, the HTTP API and MCP, without
# the GUI. Drive it with `xdl` from anywhere (XDL_API_URL + XDL_API_TOKEN),
# the browser extension through an SSH tunnel, or an agent over MCP.
#
#   docker build -t xdl .
#   docker run -d --name xdl -p 47333:47333 \
#     -e XDL_AUTH_TOKEN=<x auth_token> -e XDL_API_TOKEN=<a secret> \
#     -v xdl-data:/data -v "$PWD/downloads:/downloads" xdl

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/missuo/xdl/internal/version.Version=${VERSION}" \
      -o /out/xdl ./cmd/xdl

# xtractor is a glibc binary: Debian, not Alpine.
FROM debian:bookworm-slim
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
