package visionai

import (
	"encoding/json"
	"testing"
)

func TestNormalizeOntologyLabelsRejectsDuplicateCodeAndAttribute(t *testing.T) {
	_, err := normalizeOntologyLabels([]ontologyLabelInput{
		{Code: "car", Name: "Car", Attributes: []ontologyAttributeInput{{Name: "occluded"}}},
		{Code: "CAR", Name: "Vehicle", Attributes: []ontologyAttributeInput{{Name: "occluded"}, {Name: "Occluded"}}},
	})
	if err == nil {
		t.Fatal("duplicate category codes must be rejected case-insensitively")
	}
}

func TestNormalizeOntologyLabelsProducesStableOrder(t *testing.T) {
	labels, err := normalizeOntologyLabels([]ontologyLabelInput{
		{Code: "person", Name: "Person", Sort: 20},
		{Code: "car", Name: "Car", Sort: 10, Attributes: []ontologyAttributeInput{
			{Name: "occluded", InputType: "select", Values: []string{"yes", "yes", "no"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if labels[0].Code != "car" || len(labels[0].Attributes[0].Values) != 2 {
		t.Fatalf("unexpected normalized labels: %#v", labels)
	}
}

func TestOntologyCanonicalSnapshotIsDeterministic(t *testing.T) {
	ontology := Ontology{Code: "coco", TaskType: "CV_DETECTION"}
	version := OntologyVersion{SemanticVersion: "v1"}
	views := []ontologyLabelView{{
		OntologyLabel: OntologyLabel{Code: "person", Name: "Person", Color: "#FF4D4F", ShapeType: "rectangle"},
		Attributes: []OntologyAttribute{{
			Name: "occluded", InputType: "select", Values: `["yes","no"]`, DefaultValue: "no",
		}},
	}}
	first := ontologyCanonicalSnapshot(ontology, version, views)
	second := ontologyCanonicalSnapshot(ontology, version, views)
	if string(first) != string(second) || !json.Valid(first) {
		t.Fatalf("canonical ontology snapshot is unstable: %s", string(first))
	}
}

func TestTagDefinitionCategoriesAreGoverned(t *testing.T) {
	for _, value := range []string{"BUSINESS", "SCENE", "SOURCE"} {
		if !validTagCategory(value) {
			t.Fatalf("expected category %s to be accepted", value)
		}
	}
	if validTagCategory("FREE_TEXT") {
		t.Fatal("free-text tag category must be rejected")
	}
}
