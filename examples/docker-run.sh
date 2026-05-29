#!/usr/bin/env sh
# Example: run Synthwave Player in Docker (production-style).
#
# Prerequisites:
#   docker build -t synthwave-player-go:latest .
#   Host config at ~/.config/synthwave-player/server-config.json with
#   MUSIC_DIRECTORIES using the same paths inside the container (see mounts below).
#
# Adjust IMAGE, paths, network, and tsdproxy labels for your host.

set -e

IMAGE="${IMAGE:-princeamd/synthwave-player:3.6}"

docker run \
  --restart=always \
  --net=isolated_nw \
  --name thomas-synthwave \
  --label tsdproxy.enable=true \
  --label tsdproxy.name=synthwave-player \
  --label tsdproxy.container_port=8160 \
  --label tsdproxy.funnel=true \
  -v /mnt/share/640GB/Music_Deon:/mnt/share/640GB/Music_Deon:ro \
  -v /etc/localtime:/etc/localtime:ro \
  -v /home/thomas/.config/synthwave-player:/home/synthwave/.config/synthwave-player:rw \
  -v /home/thomas/.cache/synthwave-player:/home/synthwave/.cache/synthwave-player:rw \
  -p 0.0.0.0:8160:8160 \
  -d \
  "${IMAGE}"
