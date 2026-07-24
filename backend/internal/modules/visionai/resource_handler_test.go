package visionai

import (
	"testing"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
)

func TestIntegrationProbeTargetUsesProviderInternalURL(t *testing.T) {
	cfg := config.Config{
		CVATBaseURL:    "http://cvat-gateway:8080/",
		FiftyOneAPIURL: "http://fiftyone:5152/",
	}
	tests := []struct {
		name string
		row  IntegrationInstance
		want string
	}{
		{
			name: "cvat",
			row:  IntegrationInstance{ProviderType: "CVAT", BaseURL: "http://localhost:28080"},
			want: "http://cvat-gateway:8080/api/server/about",
		},
		{
			name: "fiftyone",
			row:  IntegrationInstance{ProviderType: "FIFTYONE", BaseURL: "http://localhost:25151"},
			want: "http://fiftyone:5152/health",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := integrationProbeTarget(test.row, cfg); got != test.want {
				t.Fatalf("target = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseGPUProbe(t *testing.T) {
	model, memoryBytes, driver, ok := parseGPUProbe("NVIDIA GeForce RTX 3060, 12288, 591.86\r\n")
	if !ok {
		t.Fatal("expected valid NVIDIA probe")
	}
	if model != "NVIDIA GeForce RTX 3060" || memoryBytes != 12288*1024*1024 || driver != "591.86" {
		t.Fatalf("unexpected probe result: %q %d %q", model, memoryBytes, driver)
	}
	if _, _, _, valid := parseGPUProbe("not-a-gpu"); valid {
		t.Fatal("invalid probe must be rejected")
	}
}
