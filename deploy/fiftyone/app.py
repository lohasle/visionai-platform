import pathlib
import re
import threading
import os
import urllib.request
from urllib.parse import urlsplit, urlunsplit

import fiftyone as fo
import httpx
import uvicorn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field


ROOT = pathlib.Path("/fiftyone/media")
ROOT.mkdir(parents=True, exist_ok=True)
api = FastAPI(title="VisionAI FiftyOne bridge", version="1.0.0")
session = None
lock = threading.Lock()
UI_URL = "http://127.0.0.1:5151"


class Detection(BaseModel):
    label: str
    confidence: float | None = None
    boundingBox: list[float] = Field(min_length=4, max_length=4)


class Sample(BaseModel):
    assetId: int
    sourceUrl: str
    split: str
    slice: str
    errorType: str
    iou: float
    latencyMs: float
    groundTruth: list[Detection] = []
    predictions: list[Detection] = []


class DatasetRequest(BaseModel):
    name: str
    projectId: int
    runId: int
    metadata: dict = {}
    samples: list[Sample]


def safe_name(value: str) -> str:
    return re.sub(r"[^a-zA-Z0-9_.-]", "_", value)


def labels(values: list[Detection], predictions: bool = False):
    rows = []
    for value in values:
        args = {"label": value.label, "bounding_box": value.boundingBox}
        if predictions:
            args["confidence"] = value.confidence
        rows.append(fo.Detection(**args))
    return fo.Detections(detections=rows)


def latest_dataset():
    datasets = [fo.load_dataset(name) for name in fo.list_datasets()]
    if not datasets:
        return None
    return max(
        datasets,
        key=lambda dataset: dataset.last_modified_at or dataset.created_at,
    )


def ensure_session(dataset=None):
    global session
    if session is None:
        session = fo.launch_app(
            dataset,
            address="0.0.0.0",
            port=5151,
            remote=True,
        )
    elif dataset is not None:
        session.dataset = dataset
    return session


def ui_ready():
    try:
        with urllib.request.urlopen(UI_URL, timeout=3) as response:
            return response.status == 200
    except Exception:
        return False


@api.get("/health")
def health():
    if not ui_ready():
        raise HTTPException(503, "FiftyOne App UI is unavailable")
    return {
        "status": "UP",
        "uiStatus": "UP",
        "fiftyoneVersion": fo.__version__,
        "dataset": session.dataset.name if session and session.dataset else None,
    }


@api.put("/datasets/{dataset_name}")
def sync_dataset(dataset_name: str, request: DatasetRequest):
    global session
    if dataset_name != request.name:
        raise HTTPException(400, "dataset name mismatch")
    with lock:
        if fo.dataset_exists(dataset_name):
            if session is not None and session.dataset is not None and session.dataset.name == dataset_name:
                session.dataset = None
            fo.delete_dataset(dataset_name)
        dataset = fo.Dataset(dataset_name, persistent=True)
        media_dir = ROOT / safe_name(dataset_name)
        media_dir.mkdir(parents=True, exist_ok=True)
        result = []
        with httpx.Client(timeout=60, follow_redirects=True) as client:
            for value in request.samples:
                target = media_dir / f"{value.assetId}.png"
                source = urlsplit(value.sourceUrl)
                media_endpoint = os.getenv("FIFTYONE_MEDIA_ENDPOINT", "")
                request_url = value.sourceUrl
                headers = {}
                if media_endpoint:
                    request_url = urlunsplit((source.scheme, media_endpoint, source.path, source.query, source.fragment))
                    headers["host"] = source.netloc
                response = client.get(request_url, headers=headers)
                response.raise_for_status()
                target.write_bytes(response.content)
                sample = fo.Sample(filepath=str(target))
                sample["asset_id"] = value.assetId
                sample["project_id"] = request.projectId
                sample["evaluation_run_id"] = request.runId
                sample["split"] = value.split
                sample["slice"] = value.slice
                sample["error_type"] = value.errorType
                sample["iou"] = value.iou
                sample["latency_ms"] = value.latencyMs
                sample["ground_truth"] = labels(value.groundTruth)
                sample["predictions"] = labels(value.predictions, True)
                result.append(sample)
        dataset.add_samples(result)
        dataset.info = request.metadata
        dataset.save()
        ensure_session(dataset)
        return {"name": dataset.name, "sampleCount": len(dataset)}


if __name__ == "__main__":
    with lock:
        ensure_session(latest_dataset())
    uvicorn.run(api, host="0.0.0.0", port=5152)
