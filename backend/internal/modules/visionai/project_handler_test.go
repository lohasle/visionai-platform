package visionai

import "testing"

func TestNormalizedRoleCodes(t *testing.T) {
	roles := normalizedRoleCodes([]string{" annotator ", "ANNOTATOR", "reviewer", ""})
	if len(roles) != 2 || roles[0] != "ANNOTATOR" || roles[1] != "REVIEWER" {
		t.Fatalf("unexpected normalized roles: %#v", roles)
	}
}

func TestInlineSecretsAreRejected(t *testing.T) {
	if !containsInlineSecret(map[string]any{"provider": map[string]any{"api_key": "plain-text"}}) {
		t.Fatal("nested API key must be treated as an inline secret")
	}
	if containsInlineSecret(map[string]any{"endpoint": "https://example.test", "region": "cn-east-1"}) {
		t.Fatal("ordinary provider configuration should be accepted")
	}
}
