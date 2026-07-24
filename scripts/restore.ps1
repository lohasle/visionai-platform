param([Parameter(Mandatory = $true)][string]$Backup)
$ErrorActionPreference = "Stop"
$resolved = (Resolve-Path -LiteralPath $Backup).Path
if (-not (Test-Path (Join-Path $resolved "mysql.sql"))) { throw "mysql.sql not found in backup." }
docker compose up -d mysql minio --wait
Get-Content -Raw (Join-Path $resolved "mysql.sql") | docker compose exec -T mysql mysql -unimbus -pnimbus_dev nimbus_platform_go
if (Test-Path (Join-Path $resolved "minio.tgz")) {
    docker run --rm --volumes-from visionai-minio -v "${resolved}:/backup:ro" alpine:3.22 tar xzf /backup/minio.tgz -C /data
}
Write-Host "Backup restored from: $resolved"
