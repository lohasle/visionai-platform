FROM python:3.11-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    FIFTYONE_DATABASE_DIR=/fiftyone/db \
    FIFTYONE_DEFAULT_DATASET_DIR=/fiftyone/datasets

RUN apt-get -o Acquire::Retries=5 update \
    && apt-get -o Acquire::Retries=5 install -y --no-install-recommends libglib2.0-0 libcurl4 \
    && rm -rf /var/lib/apt/lists/*

COPY deploy/fiftyone/requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt

COPY deploy/fiftyone/app.py /app/app.py
WORKDIR /app
EXPOSE 5151 5152
HEALTHCHECK --interval=15s --timeout=5s --start-period=90s --retries=20 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:5152/health', timeout=4)" || exit 1
CMD ["python", "/app/app.py"]
