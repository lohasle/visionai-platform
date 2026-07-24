#!/usr/bin/env sh
set -eu
backup="${1:?usage: restore.sh BACKUP_DIRECTORY}"
test -f "$backup/mysql.sql"
docker compose up -d mysql minio --wait
docker compose exec -T mysql mysql -unimbus -pnimbus_dev nimbus_platform_go <"$backup/mysql.sql"
if [ -f "$backup/minio.tgz" ]; then
  docker run --rm --volumes-from visionai-minio -v "$(cd "$backup" && pwd):/backup:ro" alpine:3.22 tar xzf /backup/minio.tgz -C /data
fi
echo "Backup restored from: $backup"
