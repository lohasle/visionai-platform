package visionai

import (
	"math"
	"testing"
)

func TestBoxIoU(t *testing.T) {
	if value := boxIoU([]float64{0, 0, 10, 10}, []float64{5, 5, 15, 15}); math.Abs(value-(25.0/175.0)) > 1e-9 {
		t.Fatalf("unexpected IoU: %f", value)
	}
	if value := boxIoU([]float64{0, 0, 10, 10}, []float64{11, 11, 15, 15}); value != 0 {
		t.Fatalf("non-overlapping boxes must have zero IoU: %f", value)
	}
}

func TestAveragePrecisionUsesConfidenceOrder(t *testing.T) {
	predictions := []classPrediction{
		{Confidence: 0.7, Matched: true},
		{Confidence: 0.9, Matched: false},
		{Confidence: 0.8, Matched: true},
	}
	value := averagePrecision(predictions, 2)
	if math.Abs(value-(2.0/3.0)) > 1e-9 {
		t.Fatalf("unexpected AP: %f", value)
	}
}

func TestScoreEvaluationSampleMatchesClassAndIoU(t *testing.T) {
	accumulator := detectionAccumulator{
		GroundTruthByClass: map[int]int{1: 1},
		PredictionsByClass: make(map[int][]classPrediction),
		ClassCode:          map[int]string{1: "person"},
	}
	row, meanIoU := scoreEvaluationSample(
		EvaluationRun{ID: 7, TenantID: 1, ProjectID: 2},
		detectionTrainingItem{AssetID: 9, Split: "TEST"},
		"night",
		[]evaluatedDetection{{ClassIndex: 1, XYXY: []float64{10, 10, 50, 50}}},
		[]evaluatedDetection{
			{ClassIndex: 1, Confidence: 0.9, XYXY: []float64{10, 10, 50, 50}},
			{ClassIndex: 2, Confidence: 0.8, XYXY: []float64{10, 10, 50, 50}},
		},
		&accumulator,
	)
	if row.ErrorType != "FP" || row.GTCount != 1 || row.PredictionCount != 2 {
		t.Fatalf("unexpected evaluation row: %+v", row)
	}
	if meanIoU != 1 || accumulator.TP != 1 || accumulator.FP != 1 || accumulator.FN != 0 {
		t.Fatalf("unexpected accumulator: %+v, IoU=%f", accumulator, meanIoU)
	}
}
