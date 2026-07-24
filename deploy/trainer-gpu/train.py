#!/usr/bin/env python3
import json
import os
from pathlib import Path
import time

import cupy as cp
import numpy as np
from PIL import Image


output_dir = Path(os.environ.get("VISIONAI_OUTPUT_DIR", "/output"))
input_dir_value = os.environ.get("VISIONAI_INPUT_DIR", "")
input_dir = Path(input_dir_value) if input_dir_value else None
run_id = os.environ["VISIONAI_RUN_ID"]
parameters = json.loads(os.environ.get("VISIONAI_PARAMETERS_JSON", "{}"))
epochs = max(3, min(int(parameters.get("epochs", 12)), 50))

device = cp.cuda.Device()
properties = cp.cuda.runtime.getDeviceProperties(device.id)
gpu_name = properties["name"].decode() if isinstance(properties["name"], bytes) else properties["name"]
free_before, total_memory = cp.cuda.runtime.memGetInfo()

image_paths = []
if input_dir:
    image_paths = sorted(
        path
        for path in (input_dir / "assets").glob("*")
        if path.suffix.lower() in {".jpg", ".jpeg", ".png", ".webp"}
    )

if image_paths:
    samples = []
    for path in image_paths:
        with Image.open(path) as image:
            samples.append(np.asarray(image.convert("L").resize((64, 64)), dtype=np.float32) / 255.0)
    host_x = np.stack(samples).reshape(len(samples), -1)
    input_mode = "staged-dataset"
else:
    # Template smoke has no governed dataset yet, but still verifies the CUDA contract.
    rng = np.random.default_rng(3060)
    host_x = rng.random((32, 4096), dtype=np.float32)
    input_mode = "gpu-template-smoke"

x = cp.asarray(host_x)
target = cp.mean(x, axis=1, keepdims=True)
weights = cp.zeros((x.shape[1], 1), dtype=cp.float32)
losses = []
started = time.perf_counter()
for _ in range(epochs):
    prediction = x @ weights
    error = prediction - target
    loss = cp.mean(error * error)
    gradient = (2.0 / x.shape[0]) * (x.T @ error)
    weights -= cp.float32(0.0005) * gradient
    cp.cuda.Stream.null.synchronize()
    losses.append(float(loss.get()))
elapsed = time.perf_counter() - started
free_after, _ = cp.cuda.runtime.memGetInfo()

output_dir.mkdir(parents=True, exist_ok=True)
np.savez(
    output_dir / "model.npz",
    weights=cp.asnumpy(weights),
    dataset_version_id=os.environ.get("VISIONAI_DATASET_MANIFEST_URI", ""),
)
metrics = {
    "epoch": list(range(1, epochs + 1)),
    "loss": losses,
    "gpu_seconds": elapsed,
    "samples": len(image_paths) if image_paths else len(host_x),
}
(output_dir / "metrics.json").write_text(json.dumps(metrics, ensure_ascii=False), encoding="utf-8")
environment = {
    "trainer": "visionai-cupy-image-regression",
    "cudaRuntime": cp.cuda.runtime.runtimeGetVersion(),
    "cudaDriver": cp.cuda.runtime.driverGetVersion(),
    "gpuModel": gpu_name,
    "gpuMemoryBytes": int(total_memory),
    "gpuMemoryUsedBytes": int(free_before - free_after),
    "inputMode": input_mode,
    "imageCount": len(image_paths),
}
(output_dir / "environment-lock.json").write_text(
    json.dumps(environment, ensure_ascii=False), encoding="utf-8"
)

result = {
    "schemaVersion": "visionai.result-manifest.v1",
    "status": "SUCCEEDED",
    "artifacts": [
        {"kind": "WEIGHTS", "path": "model.npz", "name": "model.npz", "mediaType": "application/octet-stream"},
        {"kind": "METRICS", "path": "metrics.json", "name": "metrics.json", "mediaType": "application/json"},
        {"kind": "ENVIRONMENT_LOCK", "path": "environment-lock.json", "name": "environment-lock.json", "mediaType": "application/json"},
    ],
    "metrics": [
        {"name": "train/loss", "step": index + 1, "value": value}
        for index, value in enumerate(losses)
    ] + [
        {"name": "train/gpu_seconds", "step": epochs, "value": elapsed},
        {"name": "train/images", "step": epochs, "value": float(len(image_paths))},
    ],
    "summary": {
        "primaryMetric": "train/loss",
        "primaryValue": losses[-1],
        "epochs": epochs,
        "gpuModel": gpu_name,
        "gpuMemoryBytes": int(total_memory),
        "cudaRuntime": cp.cuda.runtime.runtimeGetVersion(),
        "imageCount": len(image_paths),
        "inputMode": input_mode,
    },
}
(output_dir / "result-manifest.json").write_text(
    json.dumps(result, ensure_ascii=False), encoding="utf-8"
)
print(json.dumps(result["summary"], ensure_ascii=False))
