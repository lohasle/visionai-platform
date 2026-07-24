package visionai

import "testing"

func TestValidProjectRoles(t *testing.T) {
	if !validRoles([]string{"ANNOTATOR", "REVIEWER"}) {
		t.Fatal("known roles should be accepted")
	}
	if validRoles(nil) || validRoles([]string{"ROOT"}) {
		t.Fatal("empty or unknown roles must be rejected")
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
