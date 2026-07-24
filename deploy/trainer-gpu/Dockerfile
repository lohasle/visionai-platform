FROM nvidia/cuda:12.6.3-runtime-ubuntu24.04

ENV DEBIAN_FRONTEND=noninteractive \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_DEFAULT_TIMEOUT=120 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    CUPY_CACHE_DIR=/tmp/cupy-cache

RUN apt-get update \
    && apt-get install -y --no-install-recommends python3 python3-pip cuda-cudart-dev-12-6 \
    && rm -rf /var/lib/apt/lists/*

RUN python3 -m pip install --break-system-packages --no-cache-dir \
    cupy-cuda12x==13.6.0 numpy==2.2.6 pillow==11.3.0

RUN groupadd --gid 10001 trainer \
    && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin trainer

COPY --chmod=0555 deploy/trainer-gpu/train.py /usr/local/bin/visionai-gpu-trainer

USER 10001:10001
ENTRYPOINT ["python3", "/usr/local/bin/visionai-gpu-trainer"]
