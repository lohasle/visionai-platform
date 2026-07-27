package visionai

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	platforminference "github.com/lohasle/nimbus-framework-go/internal/platform/inference"
)

func TestValidateRegressionImage(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 4, 3))
	source.Set(1, 1, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	format, width, height, digest, err := validateRegressionImage(encoded.Bytes())
	if err != nil || format != "png" || width != 4 || height != 3 || len(digest) != 64 {
		t.Fatalf("unexpected validation result: %s %dx%d %s %v", format, width, height, digest, err)
	}
	if _, _, _, _, err = validateRegressionImage([]byte("not-an-image")); err == nil {
		t.Fatal("expected malformed image to be rejected")
	}
}

func TestEvaluateRegression(t *testing.T) {
	detections := []platforminference.Detection{
		{Label: "person", Confidence: 0.92},
		{Label: "PERSON", Confidence: 0.48},
		{Label: "car", Confidence: 0.99},
	}
	passed := evaluateRegression(detections, "Person", 0.8)
	if passed.Status != "PASSED" || passed.MatchedDetectionCount != 1 {
		t.Fatalf("unexpected passed result: %+v", passed)
	}
	failed := evaluateRegression(detections, "dog", 0.5)
	if failed.Status != "FAILED" || failed.MatchedDetectionCount != 0 {
		t.Fatalf("unexpected failed result: %+v", failed)
	}
	unasserted := evaluateRegression(detections, "", 0.5)
	if unasserted.Status != "NOT_ASSERTED" {
		t.Fatalf("unexpected unasserted result: %+v", unasserted)
	}
}

func TestBuildInferenceStatus(t *testing.T) {
	deployment := Deployment{Status: "RUNNING", CurrentRevisionID: 8, EndpointURL: "http://inference/v1"}
	revisions := []DeploymentRevision{{ID: 8, RevisionNo: 3, Status: "RUNNING", ModelVersionID: 21}}
	status := buildInferenceStatus(deployment, revisions, nil)
	if ready, ok := status["ready"].(bool); !ok || !ready {
		t.Fatalf("expected deployment to be ready: %+v", status)
	}
	revisions[0].Status = "FAILED"
	status = buildInferenceStatus(deployment, revisions, nil)
	if ready := status["ready"].(bool); ready {
		t.Fatalf("expected failed revision to be unavailable: %+v", status)
	}
}
