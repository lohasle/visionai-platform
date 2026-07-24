$ErrorActionPreference = "Stop"
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$target = Join-Path $PWD "backups\$stamp"
New-Item -ItemType Directory -Path $target -Force | Out-Null
docker compose exec -T mysql mysqldump -unimbus -pnimbus_dev --single-transaction nimbus_platform_go | Set-Content -Encoding utf8 (Join-Path $target "mysql.sql")
docker run --rm --volumes-from visionai-minio -v "${target}:/backup" alpine:3.22 tar czf /backup/minio.tgz -C /data .
docker compose config | Set-Content -Encoding utf8 (Join-Path $target "compose.resolved.yaml")
Write-Host "Backup created: $target"
