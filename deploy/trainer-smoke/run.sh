#!/bin/sh
set -eu

output_dir="${VISIONAI_OUTPUT_DIR:-/output}"
run_id="${VISIONAI_RUN_ID:?VISIONAI_RUN_ID is required}"
dataset_uri="${VISIONAI_DATASET_MANIFEST_URI:?VISIONAI_DATASET_MANIFEST_URI is required}"
parameters="${VISIONAI_PARAMETERS_JSON:-{}}"

printf '%s\n' "VisionAI LocalDocker CPU smoke run ${run_id}"
printf '%s\n' "dataset=${dataset_uri}"
printf '%s\n' "parameters=${parameters}"

fingerprint="$(printf '%s|%s|%s' "${run_id}" "${dataset_uri}" "${parameters}" | sha256sum | cut -d' ' -f1)"
printf 'VISIONAI-SMOKE-MODEL\n%s\n' "${fingerprint}" >"${output_dir}/model.bin"
printf '%s\n' '{"epoch":[1,2,3],"loss":[0.82,0.41,0.19],"mAP50":[0.31,0.58,0.76]}' >"${output_dir}/metrics.json"
printf '%s\n' '{"python":"not-used","container":"alpine-3.22.1","protocol":"visionai.result-manifest.v1"}' >"${output_dir}/environment-lock.json"

cat >"${output_dir}/result-manifest.json" <<EOF
{
  "schemaVersion": "visionai.result-manifest.v1",
  "status": "SUCCEEDED",
  "artifacts": [
    {"kind": "WEIGHTS", "path": "model.bin", "name": "model.bin", "mediaType": "application/octet-stream"},
    {"kind": "METRICS", "path": "metrics.json", "name": "metrics.json", "mediaType": "application/json"},
    {"kind": "ENVIRONMENT_LOCK", "path": "environment-lock.json", "name": "environment-lock.json", "mediaType": "application/json"}
  ],
  "metrics": [
    {"name": "train/loss", "step": 1, "value": 0.82},
    {"name": "train/loss", "step": 2, "value": 0.41},
    {"name": "train/loss", "step": 3, "value": 0.19},
    {"name": "val/mAP50", "step": 3, "value": 0.76}
  ],
  "summary": {"primaryMetric": "val/mAP50", "primaryValue": 0.76, "epochs": 3}
}
EOF

printf '%s\n' "result-manifest=/output/result-manifest.json"
