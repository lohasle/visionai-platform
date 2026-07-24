#!/usr/bin/env sh
set -eu
if [ "${CONFIRM:-}" != "YES" ]; then
  echo "Refusing destructive reset. This removes only the volumes below:"
  docker compose config --volumes
  echo "Set CONFIRM=YES and run again." >&2
  exit 2
fi
echo "Removing exactly these Compose-managed volumes:"
docker compose config --volumes
docker compose down --volumes --remove-orphans
echo "VisionAI Compose data volumes removed."
