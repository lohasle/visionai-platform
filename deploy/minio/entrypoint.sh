#!/bin/sh
set -eu
chown -R minio:minio /data
exec su-exec minio:minio minio "$@"
