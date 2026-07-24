#!/usr/bin/env sh
set -eu
"$(dirname "$0")/doctor.sh"
if [ ! -f .env ]; then
  cp .env.example .env
  echo "Created .env from .env.example."
fi
docker compose up -d --build --wait
echo "VisionAI is ready at http://localhost:48080 (admin / admin123)."
