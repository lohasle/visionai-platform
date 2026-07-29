#!/usr/bin/env python3
import json
import os
import platform
import random
import subprocess
import time
from copy import deepcopy
from pathlib import Path

import torch
import torchvision
from PIL import Image
from torch.utils.data import DataLoader, Dataset
from torchvision.models.detection import FasterRCNN_MobileNet_V3_Large_320_FPN_Weights
from torchvision.models.detection.faster_rcnn import FastRCNNPredictor
from torchvision.transforms.functional import pil_to_tensor


OUTPUT_DIR = Path(os.environ.get("VISIONAI_OUTPUT_DIR", "/output"))
INPUT_VALUE = os.environ.get("VISIONAI_INPUT_DIR", "")
INPUT_DIR = Path(INPUT_VALUE) if INPUT_VALUE else None
RUN_ID = os.environ["VISIONAI_RUN_ID"]
PARAMETERS = json.loads(os.environ.get("VISIONAI_PARAMETERS_JSON", "{}"))
COCO_NAMES = [
    "person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck", "boat",
    "traffic light", "fire hydrant", "stop sign", "parking meter", "bench", "bird", "cat",
    "dog", "horse", "sheep", "cow", "elephant", "bear", "zebra", "giraffe", "backpack",
    "umbrella", "handbag", "tie", "suitcase", "frisbee", "skis", "snowboard", "sports ball",
    "kite", "baseball bat", "baseball glove", "skateboard", "surfboard", "tennis racket",
    "bottle", "wine glass", "cup", "fork", "knife", "spoon", "bowl", "banana", "apple",
    "sandwich", "orange", "broccoli", "carrot", "hot dog", "pizza", "donut", "cake", "chair",
    "couch", "potted plant", "bed", "dining table", "toilet", "tv", "laptop", "mouse",
    "remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink",
    "refrigerator", "book", "clock", "vase", "scissors", "teddy bear", "hair drier",
    "toothbrush",
]
COCO_IDS = [
    1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23,
    24, 25, 27, 28, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 46, 47,
    48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 64, 65, 67, 70,
    72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 84, 85, 86, 87, 88, 89, 90,
]


class VisionAIDetectionDataset(Dataset):
    def __init__(self, root: Path, items: list[dict]):
        self.root = root
        self.items = items

    def __len__(self):
        return len(self.items)

    def __getitem__(self, index):
        item = self.items[index]
        with Image.open(self.root / "assets" / item["filename"]) as source:
            image = pil_to_tensor(source.convert("RGB")).float().div(255.0)
        boxes = torch.as_tensor([box["xyxy"] for box in item["boxes"]], dtype=torch.float32)
        if boxes.numel() == 0:
            boxes = torch.zeros((0, 4), dtype=torch.float32)
        labels = torch.as_tensor([box["classIndex"] for box in item["boxes"]], dtype=torch.int64)
        area = (boxes[:, 2] - boxes[:, 0]) * (boxes[:, 3] - boxes[:, 1])
        target = {
            "boxes": boxes,
            "labels": labels,
            "image_id": torch.tensor([int(item["assetId"])], dtype=torch.int64),
            "area": area,
            "iscrowd": torch.zeros((len(labels),), dtype=torch.int64),
        }
        return image, target


def collate(batch):
    return tuple(zip(*batch))


def is_coco80(classes: list[dict]) -> bool:
    return [item["name"].strip().lower() for item in classes] == COCO_NAMES


def prepare_effective_index(index: dict, pretrained: bool):
    effective = deepcopy(index)
    if not pretrained or not is_coco80(effective["classes"]):
        return effective, False
    remap = {}
    for item, coco_id in zip(effective["classes"], COCO_IDS):
        remap[int(item["index"])] = coco_id
        item["index"] = coco_id
    for item in effective.get("items", []):
        for box in item.get("boxes", []):
            box["classIndex"] = remap[int(box["classIndex"])]
    effective["classIndexPolicy"] = "torchvision-coco-category-id"
    return effective, True


def build_model(class_count: int, pretrained: bool, preserve_coco_head: bool):
    weights = FasterRCNN_MobileNet_V3_Large_320_FPN_Weights.DEFAULT if pretrained else None
    model = torchvision.models.detection.fasterrcnn_mobilenet_v3_large_320_fpn(
        weights=weights,
        weights_backbone=None,
        trainable_backbone_layers=3,
    )
    if not preserve_coco_head:
        in_features = model.roi_heads.box_predictor.cls_score.in_features
        model.roi_heads.box_predictor = FastRCNNPredictor(in_features, class_count + 1)
    return model


def smoke_dataset():
    image = torch.rand((3, 192, 192), dtype=torch.float32)
    target = {
        "boxes": torch.tensor([[32.0, 36.0, 148.0, 155.0]], dtype=torch.float32),
        "labels": torch.tensor([1], dtype=torch.int64),
        "image_id": torch.tensor([1], dtype=torch.int64),
        "area": torch.tensor([13804.0], dtype=torch.float32),
        "iscrowd": torch.tensor([0], dtype=torch.int64),
    }
    return [(image, target)]


def load_input():
    if INPUT_DIR is None or not (INPUT_DIR / "staged-index.json").exists():
        return {
            "mode": "gpu-template-smoke",
            "classes": [{"index": 1, "code": "smoke_object", "name": "Smoke Object"}],
            "datasetVersionId": 0,
            "datasetChecksum": "",
            "annotationRevisionId": 0,
            "annotationChecksum": "",
            "ontologyVersionId": 0,
            "ontologyChecksum": "",
            "items": [],
        }, smoke_dataset()
    index = json.loads((INPUT_DIR / "staged-index.json").read_text(encoding="utf-8"))
    if index.get("schemaVersion") != "visionai.detection-training-input.v1":
        raise RuntimeError("unsupported or missing detection training input contract")
    classes = index.get("classes", [])
    if not classes:
        raise RuntimeError("detection training input has no classes")
    train_items = [item for item in index["items"] if item["split"] == "TRAIN"]
    if not train_items:
        train_items = index["items"]
    if not train_items:
        raise RuntimeError("detection training input has no images")
    return index, VisionAIDetectionDataset(INPUT_DIR, train_items)


def main():
    if not torch.cuda.is_available():
        raise RuntimeError("CUDA is required by the VisionAI GPU detection trainer")
    index, _dataset = load_input()
    input_mode = index["mode"] if "mode" in index else "governed-staged-dataset"
    smoke = input_mode == "gpu-template-smoke"
    epochs = 1 if smoke else max(1, min(int(PARAMETERS.get("epochs", 3)), 50))
    batch_size = 1 if smoke else max(1, min(int(PARAMETERS.get("batchSize", 2)), 8))
    learning_rate = float(PARAMETERS.get("learningRate", 0.005))
    pretrained = bool(PARAMETERS.get("pretrained", True))
    seed = int(PARAMETERS.get("seed", 20260727))
    random.seed(seed)
    torch.manual_seed(seed)
    torch.cuda.manual_seed_all(seed)
    index, preserve_coco_head = prepare_effective_index(index, pretrained)
    class_count = len(index["classes"])
    dataset = (
        smoke_dataset()
        if smoke
        else VisionAIDetectionDataset(
            INPUT_DIR,
            [item for item in index["items"] if item["split"] == "TRAIN"] or index["items"],
        )
    )
    device = torch.device("cuda:0")
    properties = torch.cuda.get_device_properties(0)

    model = build_model(class_count, pretrained=pretrained, preserve_coco_head=preserve_coco_head).to(device)
    optimizer = torch.optim.SGD(
        [parameter for parameter in model.parameters() if parameter.requires_grad],
        lr=learning_rate,
        momentum=0.9,
        weight_decay=0.0005,
    )
    loader = (
        dataset
        if smoke
        else DataLoader(
            dataset,
            batch_size=batch_size,
            shuffle=True,
            num_workers=max(0, min(int(PARAMETERS.get("workers", 2)), 4)),
            pin_memory=True,
            collate_fn=collate,
            generator=torch.Generator().manual_seed(seed),
        )
    )
    scaler = torch.amp.GradScaler("cuda", enabled=True)
    losses = []
    component_history: dict[str, list[float]] = {}
    started = time.perf_counter()
    model.train()
    for _epoch in range(epochs):
        epoch_total = 0.0
        epoch_components: dict[str, float] = {}
        steps = 0
        for images, targets in loader:
            if smoke:
                images, targets = [images.to(device)], [{key: value.to(device) for key, value in targets.items()}]
            else:
                images = [image.to(device, non_blocking=True) for image in images]
                targets = [{key: value.to(device, non_blocking=True) for key, value in target.items()} for target in targets]
            optimizer.zero_grad(set_to_none=True)
            with torch.amp.autocast("cuda", enabled=True):
                components = model(images, targets)
                total = sum(components.values())
            scaler.scale(total).backward()
            scaler.step(optimizer)
            scaler.update()
            epoch_total += float(total.detach().cpu())
            for name, value in components.items():
                epoch_components[name] = epoch_components.get(name, 0.0) + float(value.detach().cpu())
            steps += 1
        loss = epoch_total / max(1, steps)
        losses.append(loss)
        for name, value in epoch_components.items():
            component_history.setdefault(name, []).append(value / max(1, steps))

    elapsed = time.perf_counter() - started
    torch.cuda.synchronize(device)
    peak_gpu_memory = int(torch.cuda.max_memory_allocated(device))
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    checkpoint = {
        "schemaVersion": "visionai.torchvision-detection-checkpoint.v1",
        "architecture": "fasterrcnn_mobilenet_v3_large_320_fpn",
        "stateDict": {name: tensor.detach().cpu() for name, tensor in model.state_dict().items()},
        "classes": index["classes"],
        "datasetVersionId": index.get("datasetVersionId", 0),
        "datasetChecksum": index.get("datasetChecksum", ""),
        "annotationRevisionId": index.get("annotationRevisionId", 0),
        "annotationChecksum": index.get("annotationChecksum", ""),
        "ontologyVersionId": index.get("ontologyVersionId", 0),
        "ontologyChecksum": index.get("ontologyChecksum", ""),
        "training": {
            "epochs": epochs,
            "batchSize": batch_size,
            "learningRate": learning_rate,
            "pretrainedBackbone": pretrained,
            "preservedCOCOHead": preserve_coco_head,
            "seed": seed,
        },
    }
    torch.save(checkpoint, OUTPUT_DIR / "model.pt")
    (OUTPUT_DIR / "detection-index.json").write_text(
        json.dumps(index, ensure_ascii=False), encoding="utf-8"
    )

    metrics = {
        "epoch": list(range(1, epochs + 1)),
        "loss": losses,
        "components": component_history,
        "gpuSeconds": elapsed,
        "images": len(dataset),
    }
    (OUTPUT_DIR / "metrics.json").write_text(json.dumps(metrics, ensure_ascii=False), encoding="utf-8")
    environment = {
        "trainer": "visionai-torchvision-fasterrcnn",
        "python": platform.python_version(),
        "torch": torch.__version__,
        "torchvision": torchvision.__version__,
        "cudaRuntime": torch.version.cuda,
        "cudaDriver": subprocess.check_output(
            ["nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader"],
            text=True,
        ).splitlines()[0].strip(),
        "gpuModel": properties.name,
        "gpuMemoryBytes": int(properties.total_memory),
        "peakGpuMemoryBytes": peak_gpu_memory,
        "inputMode": input_mode,
        "imageCount": len(dataset),
        "annotationCount": sum(len(item.get("boxes", [])) for item in index.get("items", [])),
    }
    (OUTPUT_DIR / "environment-lock.json").write_text(
        json.dumps(environment, ensure_ascii=False), encoding="utf-8"
    )
    artifacts = [
        {"kind": "WEIGHTS", "path": "model.pt", "name": "model.pt", "mediaType": "application/vnd.pytorch"},
        {
            "kind": "TRAINING_INDEX",
            "path": "detection-index.json",
            "name": "detection-index.json",
            "mediaType": "application/json",
        },
        {"kind": "METRICS", "path": "metrics.json", "name": "metrics.json", "mediaType": "application/json"},
        {
            "kind": "ENVIRONMENT_LOCK",
            "path": "environment-lock.json",
            "name": "environment-lock.json",
            "mediaType": "application/json",
        },
    ]
    result = {
        "schemaVersion": "visionai.result-manifest.v1",
        "status": "SUCCEEDED",
        "artifacts": artifacts,
        "metrics": [
            {"name": "train/loss", "step": step + 1, "value": value}
            for step, value in enumerate(losses)
        ]
        + [
            {"name": "train/gpu_seconds", "step": epochs, "value": elapsed},
            {"name": "train/images", "step": epochs, "value": float(len(dataset))},
            {"name": "train/gpu_peak_memory_bytes", "step": epochs, "value": float(peak_gpu_memory)},
        ],
        "summary": {
            "primaryMetric": "train/loss",
            "primaryValue": losses[-1],
            "epochs": epochs,
            "architecture": checkpoint["architecture"],
            "framework": f"torch-{torch.__version__}/torchvision-{torchvision.__version__}",
            "gpuModel": properties.name,
            "gpuMemoryBytes": int(properties.total_memory),
            "peakGpuMemoryBytes": peak_gpu_memory,
            "cudaRuntime": torch.version.cuda,
            "imageCount": len(dataset),
            "annotationCount": environment["annotationCount"],
            "inputMode": input_mode,
        },
    }
    (OUTPUT_DIR / "result-manifest.json").write_text(
        json.dumps(result, ensure_ascii=False), encoding="utf-8"
    )
    print(json.dumps(result["summary"], ensure_ascii=False))


if __name__ == "__main__":
    main()
