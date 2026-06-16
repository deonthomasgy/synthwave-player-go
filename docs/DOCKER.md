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
- MinIO/S3 credentials for the music library (`s3://music/`)
- Host config and cache bind-mounted into the container user paths
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

For S3-only deployments, set `MUSIC_DIRECTORIES` to `["s3://music/"]` (or your bucket URI) in `server-config.json`. No music volume mount is required.

### Ports

- Container listens on **8160** by default (`SERVER_PORT` in config or `--port` flag).
- Map with `-p 0.0.0.0:8160:8160` (or another host port).

### S3 / MinIO music

Pass MinIO credentials as container environment variables. When all three are set (`MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`), the server uses S3 for the library:

- On first start with no config, the entrypoint defaults `MUSIC_DIRECTORIES` to `s3://music/` (or `s3://${MINIO_BUCKET}/`).
- With an existing config, the server auto-appends the S3 URI unless `SYNTHWAVE_USE_S3_MUSIC=0`.

```bash
docker run -d --name synthwave-player \
  -p 8160:8160 \
  -e MINIO_ENDPOINT=http://truenas-scale.sphinx-butterfly.ts.net:9000 \
  -e MINIO_ACCESS_KEY=thomas \
  -e MINIO_SECRET_KEY='your-secret' \
  -e MINIO_BUCKET=music \
  -v synthwave-config:/home/synthwave/.config \
  -v synthwave-cache:/home/synthwave/.cache \
  synthwave-player-go:latest
```

S3-only mode needs no `/music` volume. Local and S3 sources can be combined by mounting local paths and setting both in `server-config.json`.
