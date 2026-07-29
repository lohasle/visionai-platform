import hashlib
import io
import json
import os
import pathlib
import tempfile
import threading
import time
import urllib.request
from typing import Any
from urllib.parse import urlparse

import imageio_ffmpeg
import torch
import torchvision
from fastapi import FastAPI, File, HTTPException, UploadFile
from minio import Minio
from PIL import Image
from pydantic import BaseModel, Field
from torchvision.models.detection.faster_rcnn import FastRCNNPredictor
from torchvision.transforms.functional import pil_to_tensor


api = FastAPI(title="VisionAI TorchVision detection inference provider", version="2.0.0")
STATE_DIR = pathlib.Path("/state")
STATE_FILE = STATE_DIR / "active.json"
ARTIFACT_DIR = STATE_DIR / "artifacts"
STATE_DIR.mkdir(parents=True, exist_ok=True)
ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
active: dict[str, Any] = json.loads(STATE_FILE.read_text()) if STATE_FILE.exists() else {}
loaded_models: dict[str, tuple[torch.nn.Module, dict[str, Any]]] = {}
model_lock = threading.RLock()
device = torch.device(
    "cuda:0"
    if os.environ.get("VISIONAI_INFERENCE_DEVICE", "cuda").lower() == "cuda"
    and torch.cuda.is_available()
    else "cpu"
)


class Revision(BaseModel):
    deploymentId: int
    revisionId: int
    modelVersionId: int
    artifactUri: str
    artifactSha256: str
    config: dict[str, Any] = Field(default_factory=dict)


class EvaluationModel(BaseModel):
    trainingRunId: int
    artifactUri: str
    artifactSha256: str


def save_active():
    STATE_FILE.write_text(json.dumps(active, ensure_ascii=False, indent=2), encoding="utf-8")


def minio_client():
    return Minio(
        os.environ.get("VISIONAI_S3_ENDPOINT", "minio:9000"),
        access_key=os.environ.get("VISIONAI_S3_ACCESS_KEY", "visionai"),
        secret_key=os.environ.get("VISIONAI_S3_SECRET_KEY", "visionai_minio_dev"),
        secure=os.environ.get("VISIONAI_S3_SECURE", "false").lower() == "true",
    )


def download_artifact(uri: str, expected_sha256: str) -> pathlib.Path:
    expected_sha256 = expected_sha256.lower()
    if len(expected_sha256) != 64:
        raise ValueError("artifact SHA-256 must contain 64 hexadecimal characters")
    destination = ARTIFACT_DIR / f"{expected_sha256}.pt"
    if destination.exists():
        digest = hashlib.sha256(destination.read_bytes()).hexdigest()
        if digest == expected_sha256:
            return destination
        destination.unlink()
    temporary = destination.with_suffix(".part")
    hasher = hashlib.sha256()
    parsed = urlparse(uri)
    if parsed.scheme == "s3":
        response = minio_client().get_object(parsed.netloc, parsed.path.lstrip("/"))
        try:
            with temporary.open("wb") as target:
                for chunk in response.stream(1024 * 1024):
                    target.write(chunk)
                    hasher.update(chunk)
        finally:
            response.close()
            response.release_conn()
    elif parsed.scheme in {"http", "https"}:
        with urllib.request.urlopen(uri, timeout=120) as response, temporary.open("wb") as target:
            while chunk := response.read(1024 * 1024):
                target.write(chunk)
                hasher.update(chunk)
    else:
        raise ValueError("artifact URI must use s3, http, or https")
    if hasher.hexdigest() != expected_sha256:
        temporary.unlink(missing_ok=True)
        raise ValueError("downloaded artifact SHA-256 does not match immutable revision")
    temporary.replace(destination)
    return destination


def build_model(class_count: int, preserve_coco_head: bool):
    model = torchvision.models.detection.fasterrcnn_mobilenet_v3_large_320_fpn(
        weights=None,
        weights_backbone=None,
    )
    if not preserve_coco_head:
        in_features = model.roi_heads.box_predictor.cls_score.in_features
        model.roi_heads.box_predictor = FastRCNNPredictor(in_features, class_count + 1)
    return model


def load_model(uri: str, artifact_sha256: str):
    with model_lock:
        if artifact_sha256 in loaded_models:
            return loaded_models[artifact_sha256]
        checkpoint_path = download_artifact(uri, artifact_sha256)
        checkpoint = torch.load(checkpoint_path, map_location="cpu", weights_only=False)
        if checkpoint.get("schemaVersion") != "visionai.torchvision-detection-checkpoint.v1":
            raise ValueError("unsupported VisionAI detection checkpoint")
        classes = checkpoint.get("classes", [])
        if not classes:
            raise ValueError("checkpoint contains no governed classes")
        model = build_model(
            len(classes),
            bool(checkpoint.get("training", {}).get("preservedCOCOHead", False)),
        )
        model.load_state_dict(checkpoint["stateDict"], strict=True)
        model.eval().to(device)
        metadata = {
            key: checkpoint.get(key)
            for key in (
                "schemaVersion",
                "architecture",
                "classes",
                "datasetVersionId",
                "datasetChecksum",
                "annotationRevisionId",
                "annotationChecksum",
                "ontologyVersionId",
                "ontologyChecksum",
                "training",
            )
        }
        loaded_models[artifact_sha256] = (model, metadata)
        return model, metadata


def infer_image(model, metadata, image: Image.Image, minimum_confidence: float):
    tensor = pil_to_tensor(image.convert("RGB")).float().div(255.0).to(device)
    with model_lock, torch.inference_mode():
        output = model([tensor])[0]
    classes = {int(item["index"]): item for item in metadata["classes"]}
    detections = []
    for box, label, score in zip(output["boxes"], output["labels"], output["scores"]):
        confidence = float(score.detach().cpu())
        if confidence < minimum_confidence:
            continue
        governed = classes.get(int(label.detach().cpu()))
        if governed is None:
            continue
        detections.append(
            {
                "label": governed["name"],
                "labelCode": governed["code"],
                "classIndex": governed["index"],
                "confidence": round(confidence, 6),
                "bbox": [round(float(value), 3) for value in box.detach().cpu().tolist()],
                "bboxFormat": "xyxy",
            }
        )
    return detections


def prediction_payload(model, metadata, model_version_id, revision_id, raw, minimum_confidence):
    started = time.perf_counter()
    try:
        image = Image.open(io.BytesIO(raw))
        image.load()
    except Exception as exc:
        raise HTTPException(422, f"invalid image: {exc}")
    detections = infer_image(model, metadata, image, minimum_confidence)
    return {
        "modelVersionId": model_version_id,
        "revisionId": revision_id,
        "image": {
            "width": image.width,
            "height": image.height,
            "sha256": hashlib.sha256(raw).hexdigest(),
        },
        "detections": detections,
        "latencyMs": round((time.perf_counter() - started) * 1000, 3),
        "engine": "TORCHVISION_FASTER_RCNN",
        "device": str(device),
        "ontologyChecksum": metadata.get("ontologyChecksum"),
    }


@api.on_event("startup")
def restore_active_model():
    if active:
        load_model(active["artifactUri"], active["artifactSha256"])


@api.get("/health")
def health():
    return {
        "status": "UP",
        "activeRevision": active.get("revisionId"),
        "engine": "TORCHVISION_FASTER_RCNN",
        "device": str(device),
        "gpu": torch.cuda.get_device_name(device) if device.type == "cuda" else None,
        "torch": torch.__version__,
        "torchvision": torchvision.__version__,
    }


@api.put("/admin/revisions/{revision_id}")
def activate(revision_id: int, revision: Revision):
    if revision_id != revision.revisionId:
        raise HTTPException(400, "revision ID does not match immutable request")
    try:
        _model, metadata = load_model(revision.artifactUri, revision.artifactSha256)
    except Exception as exc:
        raise HTTPException(422, f"model activation failed: {exc}")
    active.clear()
    active.update(revision.model_dump())
    active["ontologyChecksum"] = metadata.get("ontologyChecksum")
    save_active()
    return {
        "status": "RUNNING",
        "revisionId": revision_id,
        "engine": "TORCHVISION_FASTER_RCNN",
        "device": str(device),
        "ontologyChecksum": metadata.get("ontologyChecksum"),
    }


@api.delete("/admin/revisions/{revision_id}")
def stop(revision_id: int):
    if active.get("revisionId") == revision_id:
        active.clear()
        save_active()
    return {"status": "STOPPED", "revisionId": revision_id}


@api.put("/admin/evaluations/{evaluation_run_id}")
def load_evaluation_model(evaluation_run_id: int, request: EvaluationModel):
    if evaluation_run_id <= 0 or request.trainingRunId <= 0:
        raise HTTPException(400, "evaluation and training run IDs are required")
    try:
        _model, metadata = load_model(request.artifactUri, request.artifactSha256)
    except Exception as exc:
        raise HTTPException(422, f"evaluation model load failed: {exc}")
    return {
        "status": "READY",
        "evaluationRunId": evaluation_run_id,
        "trainingRunId": request.trainingRunId,
        "artifactSha256": request.artifactSha256,
        "ontologyChecksum": metadata.get("ontologyChecksum"),
    }


@api.post("/v1/evaluations/{evaluation_run_id}/predict")
async def predict_evaluation(
    evaluation_run_id: int,
    training_run_id: int,
    artifact_sha256: str,
    file: UploadFile = File(...),
):
    if evaluation_run_id <= 0 or training_run_id <= 0:
        raise HTTPException(400, "evaluation and training run IDs are required")
    loaded = loaded_models.get(artifact_sha256)
    if loaded is None:
        raise HTTPException(409, "evaluation model is not loaded")
    raw = await file.read()
    model, metadata = loaded
    return prediction_payload(model, metadata, training_run_id, evaluation_run_id, raw, 0.001)


@api.post("/v1/predict")
async def predict(file: UploadFile = File(...)):
    if not active:
        raise HTTPException(503, "no active deployment revision")
    raw = await file.read()
    model, metadata = load_model(active["artifactUri"], active["artifactSha256"])
    minimum_confidence = float(active.get("config", {}).get("minimumConfidence", 0.25))
    return prediction_payload(
        model,
        metadata,
        active["modelVersionId"],
        active["revisionId"],
        raw,
        minimum_confidence,
    )


@api.post("/v1/predict-video")
async def predict_video(file: UploadFile = File(...)):
    if not active:
        raise HTTPException(503, "no active deployment revision")
    started = time.perf_counter()
    raw = await file.read()
    suffix = pathlib.Path(file.filename or "video.mp4").suffix or ".mp4"
    model, metadata = load_model(active["artifactUri"], active["artifactSha256"])
    threshold = float(active.get("config", {}).get("minimumConfidence", 0.25))
    frames = []
    width = height = 0
    with tempfile.NamedTemporaryFile(suffix=suffix) as handle:
        handle.write(raw)
        handle.flush()
        try:
            reader = imageio_ffmpeg.read_frames(handle.name)
            video_metadata = next(reader)
            width, height = video_metadata["size"]
            fps = max(float(video_metadata.get("fps", 1)), 1)
            for index, frame in enumerate(reader):
                if index >= 10:
                    break
                image = Image.frombytes("RGB", (width, height), frame)
                frames.append(
                    {
                        "frameIndex": index,
                        "timestampMs": round(index * 1000 / fps, 2),
                        "detections": infer_image(model, metadata, image, threshold),
                    }
                )
        except Exception as exc:
            raise HTTPException(422, f"invalid video: {exc}")
    return {
        "modelVersionId": active["modelVersionId"],
        "revisionId": active["revisionId"],
        "video": {
            "width": width,
            "height": height,
            "sampledFrames": len(frames),
            "sha256": hashlib.sha256(raw).hexdigest(),
        },
        "frames": frames,
        "latencyMs": round((time.perf_counter() - started) * 1000, 3),
        "engine": "TORCHVISION_FASTER_RCNN_VIDEO",
        "device": str(device),
    }
