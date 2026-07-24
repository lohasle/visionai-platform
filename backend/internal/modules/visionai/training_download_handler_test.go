package visionai

import "testing"

func TestTrainingStorageKey(t *testing.T) {
	key, err := trainingStorageKey(
		"s3://visionai/tenants/7/projects/9/training/runs/12/artifacts/best.onnx",
		"visionai",
		"tenants/7/projects/9/training/runs/12",
	)
	if err != nil || key != "tenants/7/projects/9/training/runs/12/artifacts/best.onnx" {
		t.Fatalf("expected valid key, got %q, %v", key, err)
	}
	for _, uri := range []string{
		"https://example.com/model.onnx",
		"s3://other/tenants/7/projects/9/training/runs/12/artifacts/best.onnx",
		"s3://visionai/tenants/7/projects/10/training/runs/12/artifacts/best.onnx",
		"s3://visionai/tenants/7/projects/9/training/runs/123/artifacts/best.onnx",
	} {
		if _, err := trainingStorageKey(uri, "visionai", "tenants/7/projects/9/training/runs/12"); err == nil {
			t.Fatalf("expected URI %q to be rejected", uri)
		}
	}
}

func TestTrainingExportEntry(t *testing.T) {
	if got := trainingExportEntry("MODEL", "best.onnx", 31); got != "artifacts/31-best.onnx" {
		t.Fatalf("unexpected artifact entry %q", got)
	}
	if got := trainingExportEntry("LOG", "stdout.log", 32); got != "logs/32-stdout.log" {
		t.Fatalf("unexpected log entry %q", got)
	}
	if got := trainingExportEntry("MODEL", "../unsafe", 33); got != "artifacts/33-artifact-33.bin" {
		t.Fatalf("unexpected fallback entry %q", got)
	}
}
