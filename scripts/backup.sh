#!/usr/bin/env sh
set -eu
stamp="$(date +%Y%m%d-%H%M%S)"
target="${1:-backups/$stamp}"
mkdir -p "$target"
docker compose exec -T mysql mysqldump -unimbus -pnimbus_dev --single-transaction nimbus_platform_go >"$target/mysql.sql"
docker run --rm --volumes-from visionai-minio -v "$(cd "$target" && pwd):/backup" alpine:3.22 tar czf /backup/minio.tgz -C /data .
docker compose config >"$target/compose.resolved.yaml"
echo "Backup created: $target"
