# NOVA İndirici — web sürümü
FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY web ./web
ARG NOVA_VERSION=1.5.0
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.AppVersion=${NOVA_VERSION}" -o /out/nova .

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*
RUN useradd --create-home --uid 10001 nova \
 && mkdir -p /data/bin \
 && curl -fsSL -o /data/bin/yt-dlp https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux \
 && chmod 755 /data/bin/yt-dlp \
 && chown -R nova:nova /data
COPY --from=build /out/nova /usr/local/bin/nova
USER nova
ENV NOVA_SERVER=1 \
    NOVA_DATA=/data \
    PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD curl -fs http://127.0.0.1:${PORT}/healthz || exit 1
CMD ["nova"]
