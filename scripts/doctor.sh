#!/usr/bin/env sh
set -eu
command -v docker >/dev/null
docker version >/dev/null
docker compose version >/dev/null
docker compose config --quiet
free_kb="$(df -Pk . | awk 'NR==2 {print $4}')"
if [ "$free_kb" -lt 20971520 ]; then
  echo "WARNING: less than 20 GB free disk space remains." >&2
fi
echo "VisionAI doctor: Docker, Compose, configuration and disk checks passed."
