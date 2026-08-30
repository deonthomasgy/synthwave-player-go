#!/usr/bin/env sh
# Example: run Synthwave Player in Docker (production-style).
#
# Prerequisites:
#   docker build -t synthwave-player-go:latest .
#   Host config at ~/.config/synthwave-player/server-config.json with
#   MUSIC_DIRECTORIES set to s3://music/ (or your bucket URI).
#
# Adjust IMAGE, paths, network, and tsdproxy labels for your host.

set -e

IMAGE="${IMAGE:-princeamd/synthwave-player:3.6}"

# MinIO/S3 music (optional). When set, the server auto-adds s3://music/ to MUSIC_DIRECTORIES
# unless SYNTHWAVE_USE_S3_MUSIC=0. Set MINIO_SECRET_KEY in your shell or a .env file.
MINIO_ENDPOINT="${MINIO_ENDPOINT:-http://mini-io.sphinx-butterfly.ts.net:9000}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-thomas}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:?Set MINIO_SECRET_KEY before running}"
MINIO_BUCKET="${MINIO_BUCKET:-music}"

docker run \
  --restart=always \
  --net=isolated_nw \
  --name thomas-synthwave \
  --label tsdproxy.enable=true \
  --label tsdproxy.name=synthwave-player \
  --label tsdproxy.container_port=8160 \
  --label tsdproxy.funnel=true \
  -e MINIO_ENDPOINT="${MINIO_ENDPOINT}" \
  -e MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY}" \
  -e MINIO_SECRET_KEY="${MINIO_SECRET_KEY}" \
  -e MINIO_BUCKET="${MINIO_BUCKET}" \
  -v /etc/localtime:/etc/localtime:ro \
  -v /home/thomas/.config/synthwave-player:/home/synthwave/.config/synthwave-player:rw \
  -v /home/thomas/.cache/synthwave-player:/home/synthwave/.cache/synthwave-player:rw \
  -p 0.0.0.0:8160:8160 \
  -d \
  "${IMAGE}"
