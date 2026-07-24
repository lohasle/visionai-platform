package visionai

import "testing"

func TestValidRelativeImportPath(t *testing.T) {
	for _, value := range []string{"../outside", `..\outside`, "/absolute", `C:\outside`, ""} {
		if validRelativeImportPath(value) {
			t.Fatalf("unsafe import path %q was accepted", value)
		}
	}
	for _, value := range []string{"camera-a", "line/shift-1"} {
		if !validRelativeImportPath(value) {
			t.Fatalf("safe import path %q was rejected", value)
		}
	}
}
