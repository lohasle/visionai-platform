package visionai

import (
	"testing"

	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
)

func TestNormalizeTrainingParametersAppliesDefaultsAndValidatesSchema(t *testing.T) {
	schema := `{
		"type":"object",
		"required":["epochs","device"],
		"additionalProperties":false,
		"properties":{
			"epochs":{"type":"integer","minimum":1,"maximum":20},
			"device":{"type":"string","enum":["GPU","CPU"],"default":"GPU"},
			"pretrained":{"type":"boolean","default":true}
		}
	}`
	parameters, err := normalizeTrainingParameters(schema, map[string]any{"epochs": float64(2)})
	if err != nil {
		t.Fatalf("expected valid parameters, got %v", err)
	}
	if parameters["device"] != "GPU" || parameters["pretrained"] != true {
		t.Fatalf("schema defaults were not applied: %#v", parameters)
	}

	if _, err = normalizeTrainingParameters(schema, map[string]any{"epochs": float64(0)}); err == nil {
		t.Fatal("minimum constraint must reject zero epochs")
	}
	if _, err = normalizeTrainingParameters(schema, map[string]any{"epochs": 2.5}); err == nil {
		t.Fatal("integer constraint must reject fractional values")
	}
	if _, err = normalizeTrainingParameters(schema, map[string]any{"epochs": float64(2), "unknown": true}); err == nil {
		t.Fatal("additionalProperties=false must reject unknown parameters")
	}
}

func TestLocalDockerProductionBoundary(t *testing.T) {
	if !localDockerAllowed(config.Config{Environment: "DEVELOPMENT"}) {
		t.Fatal("development must allow LocalDocker")
	}
	if localDockerAllowed(config.Config{Environment: "PRODUCTION"}) {
		t.Fatal("production must reject LocalDocker unless explicitly enabled")
	}
	if !localDockerAllowed(config.Config{
		Environment: "PRODUCTION", EnableProductionLocalDocker: true,
	}) {
		t.Fatal("explicit production Linux provider enablement must be honored")
	}
}
