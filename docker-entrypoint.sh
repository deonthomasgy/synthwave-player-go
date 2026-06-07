#!/bin/sh
set -e

CONFIG_DIR="${XDG_CONFIG_HOME}/synthwave-player"
CONFIG_FILE="${CONFIG_DIR}/server-config.json"

mkdir -p "${CONFIG_DIR}" "${XDG_CACHE_HOME}" 2>/dev/null || {
    export XDG_CONFIG_HOME="/home/synthwave/.config"
    export XDG_CACHE_HOME="/home/synthwave/.cache"
    CONFIG_DIR="${XDG_CONFIG_HOME}/synthwave-player"
    CONFIG_FILE="${CONFIG_DIR}/server-config.json"
    mkdir -p "${CONFIG_DIR}" "${XDG_CACHE_HOME}"
}

if [ ! -f "${CONFIG_FILE}" ]; then
    if [ -n "${MINIO_ENDPOINT}" ] && [ -n "${MINIO_ACCESS_KEY}" ] && [ -n "${MINIO_SECRET_KEY}" ]; then
        BUCKET="${MINIO_BUCKET:-music}"
        cat > "${CONFIG_FILE}" <<EOF
{
  "MUSIC_DIRECTORIES": ["s3://${BUCKET}/"],
  "SERVER_HOST": "0.0.0.0",
  "SERVER_PORT": 8160,
  "WEBSITE_TITLE": "Synthwave Player"
}
EOF
    else
        cat > "${CONFIG_FILE}" <<'EOF'
{
  "MUSIC_DIRECTORIES": ["/music"],
  "SERVER_HOST": "0.0.0.0",
  "SERVER_PORT": 8160,
  "WEBSITE_TITLE": "Synthwave Player"
}
EOF
    fi
fi

exec /usr/lib/synthwave-player/synthwave-player-server "$@"
