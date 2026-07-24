package visionai

import "testing"

func TestSafeArchiveEntryRejectsZipSlip(t *testing.T) {
	for _, value := range []string{"../secret.png", `/etc/passwd`, `..\secret.png`, "."} {
		if _, ok := safeArchiveEntry(value); ok {
			t.Fatalf("unsafe archive entry %q accepted", value)
		}
	}
	if name, ok := safeArchiveEntry("camera/day-1/frame.png"); !ok || name != "frame.png" {
		t.Fatalf("safe archive entry rejected: %q", name)
	}
}
