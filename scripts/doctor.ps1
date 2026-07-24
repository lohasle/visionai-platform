$ErrorActionPreference = "Stop"
$required = @("docker")
foreach ($command in $required) {
    if (-not (Get-Command $command -ErrorAction SilentlyContinue)) {
        throw "Missing required command: $command"
    }
}
docker version | Out-Null
docker compose version | Out-Null
docker compose config --quiet
$drive = Get-PSDrive -Name ([System.IO.Path]::GetPathRoot($PWD.Path).TrimEnd(":\"))
if ($drive.Free -lt 20GB) {
    Write-Warning "Less than 20 GB free disk space remains."
}
Write-Host "VisionAI doctor: Docker, Compose, configuration and disk checks passed."
