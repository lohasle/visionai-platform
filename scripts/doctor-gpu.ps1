$ErrorActionPreference = "Stop"
& "$PSScriptRoot\doctor.ps1"
if (-not (Get-Command nvidia-smi -ErrorAction SilentlyContinue)) {
    throw "nvidia-smi not found. Install an NVIDIA driver and Docker Desktop GPU support."
}
nvidia-smi | Out-Null
docker run --rm --gpus all nvidia/cuda:12.8.1-base-ubuntu24.04 nvidia-smi | Out-Null
Write-Host "VisionAI GPU doctor: host and container GPU checks passed."
