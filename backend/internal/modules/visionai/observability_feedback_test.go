package visionai

import (
	"errors"
	"testing"
	"time"
)

func TestParseGPUProbeMetrics(t *testing.T) {
	model, total, driver, used, utilization, temperature, power, ok := parseGPUProbeMetrics(
		"NVIDIA GeForce RTX 3060, 12288, 591.86, 2048, 73, 64, 128.5\r\n",
	)
	if !ok || model != "NVIDIA GeForce RTX 3060" || driver != "591.86" {
		t.Fatalf("unexpected probe identity: %q %q %t", model, driver, ok)
	}
	if total != 12288*1024*1024 || used != 2048*1024*1024 ||
		utilization != 73 || temperature != 64 || power != 128.5 {
		t.Fatalf("unexpected probe metrics: %d %d %.1f %.1f %.1f", total, used, utilization, temperature, power)
	}
}

func TestParseHumanBytes(t *testing.T) {
	if got := parseHumanBytes("15.54GiB"); got != 16685947944 {
		t.Fatalf("GiB conversion = %d", got)
	}
	if got := parseHumanBytes("8.41MB"); got != 8_410_000 {
		t.Fatalf("MB conversion = %d", got)
	}
}

func TestPerceptualHashNearDuplicate(t *testing.T) {
	if !perceptualHashesSimilar("0000000000000000", "000000000000003f") {
		t.Fatal("six-bit distance should be treated as a near duplicate")
	}
	if perceptualHashesSimilar("0000000000000000", "ffffffffffffffff") {
		t.Fatal("dissimilar hashes must not be deduplicated")
	}
	if perceptualHashesSimilar("sha256", "sha256-other") {
		t.Fatal("non-perceptual hashes must only match exactly")
	}
}

func TestCalculateDriftMetricsUsesBaselineWindow(t *testing.T) {
	now := time.Now()
	baseline := DriftBaseline{
		ID: 1, WindowMinutes: 60, SampleCount: 1,
		MetricsSnapshot: `{"meanConfidence":0.9,"emptyRate":0}`,
		ClassSnapshot:   `{"cat":1}`,
	}
	traces := []InferenceTrace{{
		Status: "SUCCEEDED", MeanConfidence: 0.5, DetectionCount: 1,
		Result: `{"detections":[{"label":"dog"}]}`, CreatedAt: now,
	}}
	result := calculateDriftMetrics(baseline, traces)
	if result["status"] != "DRIFTED" {
		t.Fatalf("status = %v, want DRIFTED", result["status"])
	}
	if result["windowMinutes"] != 60 || result["sampleCount"] != 1 {
		t.Fatalf("window evidence is incomplete: %#v", result)
	}
}

func TestFeedbackBenefitDeltas(t *testing.T) {
	deltas := feedbackBenefitDeltas(
		map[string]float64{"mAP": .5, "precision": .6, "recall": .4},
		map[string]float64{"mAP": .6, "precision": .65, "recall": .5},
		map[string]float64{"p95": 100, "errorRate": .1},
		map[string]float64{"p95": 90, "errorRate": .02},
	)
	if deltas["mAP"] < .099 || deltas["p95"] != -10 || deltas["errorRate"] != -.08 {
		t.Fatalf("unexpected benefit deltas: %#v", deltas)
	}
}

func TestRestoredAssetStatusPreservesQualityGate(t *testing.T) {
	if got := restoredAssetStatus(Asset{RecycleFromStatus: AssetInvalid}); got != AssetInvalid {
		t.Fatalf("invalid asset restored as %s", got)
	}
	if got := restoredAssetStatus(Asset{ErrorCode: "SENSITIVE_FIELD"}); got != AssetInvalid {
		t.Fatalf("legacy invalid asset restored as %s", got)
	}
	if got := restoredAssetStatus(Asset{RecycleFromStatus: AssetReady}); got != AssetReady {
		t.Fatalf("ready asset restored as %s", got)
	}
}

func TestAssetMetadataStringDoesNotPersistNilSentinel(t *testing.T) {
	if got := assetMetadataString(map[string]any{}, "businessScene"); got != "" {
		t.Fatalf("missing metadata became %q", got)
	}
	if got := assetMetadataString(map[string]any{"businessScene": nil}, "businessScene"); got != "" {
		t.Fatalf("nil metadata became %q", got)
	}
	if got := assetMetadataString(map[string]any{"businessScene": " inspection "}, "businessScene"); got != "inspection" {
		t.Fatalf("metadata normalization returned %q", got)
	}
}

func TestAssetQualityErrorClassification(t *testing.T) {
	cases := map[string]string{
		"empty text asset":                          "EMPTY_TEXT",
		"sensitive field detected in text asset":    "SENSITIVE_FIELD",
		"text asset is not valid UTF-8":             "TEXT_DECODE_FAILED",
		"unsupported media format":                  "FORMAT_MISSING",
		"decode full image: unexpected end of file": "MEDIA_DECODE_FAILED",
	}
	for message, expected := range cases {
		if actual := assetInspectionErrorCode(errors.New(message)); actual != expected {
			t.Fatalf("%q: expected %s, got %s", message, expected, actual)
		}
	}
	if !isTextAsset("manifest.jsonl", "") || !isTextAsset("notes.bin", "text/plain") {
		t.Fatal("text assets must be recognized by extension or content type")
	}
}

func TestModelProductionEligibilityAcrossDeploymentLifecycle(t *testing.T) {
	now := time.Now()
	for _, status := range []string{"APPROVED", "STAGING", "CANARY", "PRODUCTION"} {
		if !modelProductionEligible(ModelVersion{Status: status, ApprovedAt: &now}) {
			t.Fatalf("approved model in %s lifecycle state must remain production eligible", status)
		}
	}
	if modelProductionEligible(ModelVersion{Status: "EVALUATED"}) ||
		modelProductionEligible(ModelVersion{Status: "PRODUCTION"}) {
		t.Fatal("models without an approval timestamp must not be production eligible")
	}
}

func TestAssetPurgeEligibilityUsesRecycleRetention(t *testing.T) {
	deletedAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	asset := Asset{Status: AssetDeleted, DeletedAt: &deletedAt}
	eligibleAt := assetPurgeEligibleAt(asset, 30)
	want := deletedAt.Add(30 * 24 * time.Hour)
	if eligibleAt == nil || !eligibleAt.Equal(want) {
		t.Fatalf("purge eligibility = %v, want %v", eligibleAt, want)
	}
	if assetPurgeEligibleAt(Asset{Status: AssetReady}, 30) != nil {
		t.Fatal("active assets must never receive a purge eligibility timestamp")
	}
}
