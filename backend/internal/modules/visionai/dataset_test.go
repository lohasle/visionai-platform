package visionai

import "testing"

func TestDeterministicDatasetSplit(t *testing.T) {
	split, ok := normalizeSplit(map[string]float64{"TRAIN": 0.8, "VAL": 0.1, "TEST": 0.1})
	if !ok {
		t.Fatal("valid split rejected")
	}
	first := splitFor(20260721, 42, split)
	for range 10 {
		if got := splitFor(20260721, 42, split); got != first {
			t.Fatalf("split is not deterministic: %s != %s", got, first)
		}
	}
	if _, ok = normalizeSplit(map[string]float64{"TRAIN": 0.8, "VAL": 0.2, "TEST": 0.2}); ok {
		t.Fatal("split ratios above 1 must be rejected")
	}
}
