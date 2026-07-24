package visionai

import "testing"

func TestAnnotationStateMachineHappyPathAndRework(t *testing.T) {
	task := AnnotationTask{Status: AnnotationDraft}
	happy := []AnnotationStatus{
		AnnotationPreparing, AnnotationReady, AnnotationAnnotating, AnnotationReviewing,
		AnnotationRejected, AnnotationAnnotating, AnnotationReviewing, AnnotationApproved,
		AnnotationExporting, AnnotationClosed,
	}
	for _, target := range happy {
		if !transitionAnnotation(&task, target) {
			t.Fatalf("transition %s -> %s rejected", task.Status, target)
		}
	}
	if transitionAnnotation(&task, AnnotationAnnotating) {
		t.Fatal("closed annotation task must be immutable")
	}
}

func TestAnnotationStateMachineRecovery(t *testing.T) {
	task := AnnotationTask{Status: AnnotationPreparing}
	if !transitionAnnotation(&task, AnnotationFailed) || !transitionAnnotation(&task, AnnotationPreparing) {
		t.Fatal("failed CVAT preparation must be recoverable")
	}
}
