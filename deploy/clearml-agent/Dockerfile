FROM docker:27-cli AS docker-cli

FROM python:3.12-slim

COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker

RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates curl git \
    && python -m pip install --no-cache-dir clearml-agent==3.0.3 \
    && rm -rf /var/lib/apt/lists/*

ENV PYTHONUNBUFFERED=1 \
    CLEARML_AGENT_SKIP_PIP_VENV_INSTALL=1 \
    CLEARML_AGENT_SKIP_PYTHON_ENV_INSTALL=1

COPY --chmod=0555 deploy/clearml-agent/entrypoint.sh /usr/local/bin/visionai-clearml-agent-entrypoint

ENTRYPOINT ["/usr/local/bin/visionai-clearml-agent-entrypoint"]
