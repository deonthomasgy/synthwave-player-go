# syntax=docker/dockerfile:1

# ---- Build ----
FROM golang:1.27.3-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/synthwave-player-server ./cmd/synthwave-player-server

# ---- Runtime ----
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        file \
        eyed3 \
        exiftool \
    && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /usr/sbin/nologin --uid 1000 synthwave \
    && mkdir -p /music /home/synthwave/.config/synthwave-player /home/synthwave/.cache \
    && chown -R synthwave:synthwave /home/synthwave /music

COPY --from=builder /out/synthwave-player-server /usr/lib/synthwave-player/synthwave-player-server
COPY tree/usr/share/synthwave-player/public /usr/share/synthwave-player/public
COPY tree/usr/share/synthwave-player/templates /usr/share/synthwave-player/templates
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod 755 /usr/lib/synthwave-player/synthwave-player-server /usr/local/bin/docker-entrypoint.sh \
    && chown -R synthwave:synthwave /usr/share/synthwave-player

ENV SYNTHWAVE_BASE=/usr/share/synthwave-player \
    XDG_CONFIG_HOME=/home/synthwave/.config \
    XDG_CACHE_HOME=/home/synthwave/.cache

VOLUME ["/music", "/home/synthwave/.config", "/home/synthwave/.cache"]

EXPOSE 8160

USER synthwave
WORKDIR /home/synthwave

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["--host", "0.0.0.0", "--foreground"]
