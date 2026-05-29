# Docker

## Build

```bash
docker build -t synthwave-player-go:latest .
```

## Run (minimal)

```bash
docker run -d --name synthwave-player \
  -p 8160:8160 \
  -v /path/to/music:/music:ro \
  -v synthwave-config:/home/synthwave/.config \
  -v synthwave-cache:/home/synthwave/.cache \
  synthwave-player-go:latest
```

On first start, the entrypoint creates `/home/synthwave/.config/synthwave-player/server-config.json` with `MUSIC_DIRECTORIES: ["/music"]` if none exists.

## Run (production example)

See [`examples/docker-run.sh`](../examples/docker-run.sh) for a full example with:

- Tailscale/tsdproxy labels for HTTPS funnel exposure
- Music library mounted at the same path inside the container (`/mnt/share/640GB/Music_Deon`)
- Host config and cache bind-mounted into the container user paths
- Host `server-config.json` and cache bind mounts (same layout as a native install)
- `restart=always` and a custom Docker network

```bash
chmod +x examples/docker-run.sh
./examples/docker-run.sh
```

Override the image when using a local build:

```bash
IMAGE=synthwave-player-go:latest ./examples/docker-run.sh
```

### Config paths

| Host | Container |
|------|-----------|
| `~/.config/synthwave-player/` | `/home/synthwave/.config/synthwave-player/` |
| `~/.cache/synthwave-player/` | `/home/synthwave/.cache/synthwave-player/` |

`MUSIC_DIRECTORIES` in `server-config.json` must use **container** paths (e.g. `/mnt/share/640GB/Music_Deon`, not a host-only path).

### Ports

- Container listens on **8160** by default (`SERVER_PORT` in config or `--port` flag).
- Map with `-p 0.0.0.0:8160:8160` (or another host port).
