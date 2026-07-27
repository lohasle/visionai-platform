FROM pytorch/pytorch:2.7.1-cuda12.6-cudnn9-runtime

ENV DEBIAN_FRONTEND=noninteractive \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_DEFAULT_TIMEOUT=120 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    TORCH_HOME=/opt/torch-cache

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --no-cache-dir pillow==11.3.0 \
    && python -c "from torchvision.models.detection import FasterRCNN_MobileNet_V3_Large_320_FPN_Weights as W; W.DEFAULT.get_state_dict(progress=True, check_hash=True)"

RUN groupadd --gid 10001 trainer \
    && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin trainer

COPY --chmod=0555 deploy/trainer-gpu/train.py /usr/local/bin/visionai-gpu-trainer
RUN chmod -R a+rX /opt/torch-cache

USER 10001:10001
ENTRYPOINT ["python", "/usr/local/bin/visionai-gpu-trainer"]
