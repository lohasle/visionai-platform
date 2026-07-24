$ErrorActionPreference = "Stop"
if ($env:CONFIRM -ne "YES") {
    Write-Host "Refusing destructive reset. This removes only the volumes below:"
    docker compose config --volumes
    throw 'Set CONFIRM=YES and run again.'
}
Write-Host "Removing exactly these Compose-managed volumes:"
docker compose config --volumes
docker compose down --volumes --remove-orphans
Write-Host "VisionAI Compose data volumes removed."
