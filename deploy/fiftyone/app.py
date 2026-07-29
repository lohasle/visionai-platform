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
from PIL import Image
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


class SimilaritySample(BaseModel):
    assetId: int
    filename: str
    sourceUrl: str


class SimilarityRequest(BaseModel):
    name: str
    projectId: int
    distanceThreshold: int = Field(default=6, ge=0, le=32)
    samples: list[SimilaritySample]


def safe_name(value: str) -> str:
    return re.sub(r"[^a-zA-Z0-9_.-]", "_", value)


def media_request(value, client):
    source = urlsplit(value)
    media_endpoint = os.getenv("FIFTYONE_MEDIA_ENDPOINT", "")
    request_url = value
    headers = {}
    if media_endpoint:
        request_url = urlunsplit(
            (source.scheme, media_endpoint, source.path, source.query, source.fragment)
        )
        headers["host"] = source.netloc
    return client.get(request_url, headers=headers)


def average_hash(path):
    with Image.open(path) as image:
        pixels = list(image.convert("L").resize((8, 8), Image.Resampling.LANCZOS).getdata())
    mean = sum(pixels) / len(pixels)
    value = 0
    for pixel in pixels:
        value = (value << 1) | int(pixel >= mean)
    return f"{value:016x}"


def hamming(left, right):
    return (int(left, 16) ^ int(right, 16)).bit_count()


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
                response = media_request(value.sourceUrl, client)
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


@api.post("/similarity")
def analyze_similarity(request: SimilarityRequest):
    global session
    if not request.samples:
        raise HTTPException(400, "similarity analysis requires samples")
    with lock:
        if fo.dataset_exists(request.name):
            if session is not None and session.dataset is not None and session.dataset.name == request.name:
                session.dataset = None
            fo.delete_dataset(request.name)
        dataset = fo.Dataset(request.name, persistent=True)
        media_dir = ROOT / safe_name(request.name)
        media_dir.mkdir(parents=True, exist_ok=True)
        hashes = {}
        rows = []
        with httpx.Client(timeout=60, follow_redirects=True) as client:
            for value in request.samples:
                suffix = pathlib.Path(value.filename).suffix.lower()
                if suffix not in {".jpg", ".jpeg", ".png", ".webp", ".bmp", ".gif"}:
                    suffix = ".img"
                target = media_dir / f"{value.assetId}{suffix}"
                response = media_request(value.sourceUrl, client)
                response.raise_for_status()
                target.write_bytes(response.content)
                perceptual_hash = average_hash(target)
                hashes[value.assetId] = perceptual_hash
                sample = fo.Sample(filepath=str(target))
                sample["asset_id"] = value.assetId
                sample["project_id"] = request.projectId
                sample["perceptual_hash"] = perceptual_hash
                rows.append(sample)

        parents = {asset_id: asset_id for asset_id in hashes}

        def find(asset_id):
            while parents[asset_id] != asset_id:
                parents[asset_id] = parents[parents[asset_id]]
                asset_id = parents[asset_id]
            return asset_id

        def union(left, right):
            left_root, right_root = find(left), find(right)
            if left_root != right_root:
                parents[max(left_root, right_root)] = min(left_root, right_root)

        asset_ids = sorted(hashes)
        for index, left in enumerate(asset_ids):
            for right in asset_ids[index + 1 :]:
                if hamming(hashes[left], hashes[right]) <= request.distanceThreshold:
                    union(left, right)
        grouped = {}
        for asset_id in asset_ids:
            grouped.setdefault(find(asset_id), []).append(asset_id)
        groups = []
        group_by_asset = {}
        for values in grouped.values():
            if len(values) < 2:
                continue
            canonical = min(values)
            for asset_id in values:
                group_by_asset[asset_id] = canonical
            groups.append(
                {
                    "canonicalAssetId": canonical,
                    "assetIds": values,
                    "hashes": {str(asset_id): hashes[asset_id] for asset_id in values},
                }
            )
        for sample in rows:
            sample["near_duplicate_of"] = group_by_asset.get(sample["asset_id"])
        dataset.add_samples(rows)
        dataset.info = {
            "projectId": request.projectId,
            "distanceThreshold": request.distanceThreshold,
            "groupCount": len(groups),
            "algorithm": "64-bit perceptual average hash",
        }
        dataset.save()
        ensure_session(dataset)
        return {
            "name": request.name,
            "analyzedCount": len(rows),
            "groups": groups,
            "hashes": {str(asset_id): value for asset_id, value in hashes.items()},
        }


if __name__ == "__main__":
    with lock:
        ensure_session(latest_dataset())
    uvicorn.run(api, host="0.0.0.0", port=5152)
