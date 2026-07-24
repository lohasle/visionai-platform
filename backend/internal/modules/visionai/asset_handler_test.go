package visionai

import "testing"

func TestSafeFilenameRejectsTraversal(t *testing.T) {
	rejected := []string{"", ".", "..", "../secret.png", `..\secret.png`, "folder/image.png", "image\x00.png"}
	for _, value := range rejected {
		if _, ok := safeFilename(value); ok {
			t.Fatalf("unsafe filename %q was accepted", value)
		}
	}
	if got, ok := safeFilename("camera-01.png"); !ok || got != "camera-01.png" {
		t.Fatalf("valid filename rejected: %q", got)
	}
}

func TestAssetRootIsTenantAndProjectScoped(t *testing.T) {
	if got := assetRoot(12, 34); got != "tenants/12/projects/34" {
		t.Fatalf("unexpected object root %q", got)
	}
}
