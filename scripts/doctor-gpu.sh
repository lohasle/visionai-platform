#!/usr/bin/env sh
set -eu
"$(dirname "$0")/doctor.sh"
command -v nvidia-smi >/dev/null
nvidia-smi >/dev/null
docker run --rm --gpus all nvidia/cuda:12.8.1-base-ubuntu24.04 nvidia-smi >/dev/null
echo "VisionAI GPU doctor: host and container GPU checks passed."
