import hashlib
import io
import json
import pathlib
import time
import tempfile
from typing import Any

from fastapi import FastAPI, File, HTTPException, UploadFile
from pydantic import BaseModel
from PIL import Image
import imageio_ffmpeg


api = FastAPI(title="VisionAI FastAPI inference provider", version="1.0.0")
STATE_FILE = pathlib.Path("/state/active.json")
STATE_FILE.parent.mkdir(parents=True, exist_ok=True)
active: dict[str, Any] = json.loads(STATE_FILE.read_text()) if STATE_FILE.exists() else {}


class Revision(BaseModel):
    deploymentId: int
    revisionId: int
    modelVersionId: int
    artifactUri: str
    artifactSha256: str
    config: dict[str, Any] = {}


def save():
    STATE_FILE.write_text(json.dumps(active, ensure_ascii=False, indent=2))


@api.get("/health")
def health():
    return {"status": "UP", "activeRevision": active.get("revisionId")}


@api.put("/admin/revisions/{revision_id}")
def activate(revision_id: int, revision: Revision):
    if revision_id != revision.revisionId or not revision.artifactSha256:
        raise HTTPException(400, "invalid immutable revision")
    active.clear()
    active.update(revision.model_dump())
    save()
    return {"status": "RUNNING", "revisionId": revision_id}


@api.delete("/admin/revisions/{revision_id}")
def stop(revision_id: int):
    if active.get("revisionId") == revision_id:
        active.clear()
        save()
    return {"status": "STOPPED", "revisionId": revision_id}


@api.post("/v1/predict")
async def predict(file: UploadFile = File(...)):
    if not active:
        raise HTTPException(503, "no active deployment revision")
    started = time.perf_counter()
    raw = await file.read()
    try:
        image = Image.open(io.BytesIO(raw))
        width, height = image.size
        image.verify()
    except Exception as exc:
        raise HTTPException(422, f"invalid image: {exc}")
    digest = hashlib.sha256(raw).hexdigest()
    confidence = 0.72 + (int(digest[:4], 16) % 2500) / 10000
    detections = [{
        "label": "defect",
        "confidence": round(min(confidence, 0.99), 4),
        "bbox": [round(width * 0.18), round(height * 0.2), round(width * 0.45), round(height * 0.42)],
    }]
    latency = (time.perf_counter() - started) * 1000
    return {
        "modelVersionId": active["modelVersionId"],
        "revisionId": active["revisionId"],
        "image": {"width": width, "height": height, "sha256": digest},
        "detections": detections,
        "latencyMs": round(latency, 3),
        "engine": "VISIONAI_SMOKE",
    }


@api.post("/v1/predict-video")
async def predict_video(file: UploadFile = File(...)):
    if not active:
        raise HTTPException(503, "no active deployment revision")
    started = time.perf_counter()
    raw = await file.read()
    suffix = pathlib.Path(file.filename or "video.mp4").suffix or ".mp4"
    frames = []
    with tempfile.NamedTemporaryFile(suffix=suffix) as handle:
        handle.write(raw)
        handle.flush()
        try:
            reader = imageio_ffmpeg.read_frames(handle.name)
            metadata = next(reader)
            width, height = metadata["size"]
            for index, frame in enumerate(reader):
                if index >= 10:
                    break
                digest = hashlib.sha256(frame).hexdigest()
                confidence = 0.72 + (int(digest[:4], 16) % 2500) / 10000
                frames.append({
                    "frameIndex": index,
                    "timestampMs": round(index * 1000 / max(metadata.get("fps", 1), 1), 2),
                    "detections": [{"label": "defect", "confidence": round(min(confidence, .99), 4), "bbox": [round(width * .18), round(height * .2), round(width * .45), round(height * .42)]}],
                })
        except Exception as exc:
            raise HTTPException(422, f"invalid video: {exc}")
    return {
        "modelVersionId": active["modelVersionId"], "revisionId": active["revisionId"],
        "video": {"width": width, "height": height, "sampledFrames": len(frames), "sha256": hashlib.sha256(raw).hexdigest()},
        "frames": frames, "latencyMs": round((time.perf_counter() - started) * 1000, 3), "engine": "VISIONAI_SMOKE_VIDEO",
    }
