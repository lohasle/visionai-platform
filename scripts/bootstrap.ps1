$ErrorActionPreference = "Stop"
& "$PSScriptRoot\doctor.ps1"
if (-not (Test-Path ".env")) {
    Copy-Item -LiteralPath ".env.example" -Destination ".env"
    Write-Host "Created .env from .env.example."
}
docker compose up -d --build --wait
Write-Host "VisionAI is ready at http://localhost:48080 (admin / admin123)."
