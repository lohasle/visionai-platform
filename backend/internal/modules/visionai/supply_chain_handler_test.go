package visionai

import (
	"encoding/json"
	"testing"
)

func completeLicenseRequests() []licenseDeclarationRequest {
	rows := make([]licenseDeclarationRequest, 0, len(requiredLicenseComponents))
	for _, component := range requiredLicenseComponents {
		rows = append(rows, licenseDeclarationRequest{
			ComponentType: component, ComponentName: component, LicenseID: "Apache-2.0",
			UseDeclaration: "受控训练与生产部署", Decision: "ALLOWED",
		})
	}
	return rows
}

func TestNormalizeLicenseDeclarationsRequiresFullSupplyChain(t *testing.T) {
	rows := completeLicenseRequests()
	normalized, err := normalizeLicenseDeclarations(rows)
	if err != nil || len(normalized) != 4 {
		t.Fatalf("complete declarations rejected: rows=%#v err=%v", normalized, err)
	}
	if _, err = normalizeLicenseDeclarations(rows[:3]); err == nil {
		t.Fatal("missing model-artifact declaration must be rejected")
	}
	rows[2].Decision = "NOT_ALLOWED"
	if aggregateLicenseDecision(rows) != "NOT_ALLOWED" {
		t.Fatal("a blocked component must block the aggregate license decision")
	}
}

func TestVersionMatchesCompatibilityRanges(t *testing.T) {
	tests := []struct {
		version, expression string
		want                bool
	}{
		{"2.4.1", ">=2.0.0 <3.0.0", true},
		{"3.0.0", ">=2.0.0 <3.0.0", false},
		{"2.4.1", "2.x", true},
		{"1.3.0", "=1.3.0", true},
		{"invalid", "*", false},
	}
	for _, test := range tests {
		if got := versionMatches(test.version, test.expression); got != test.want {
			t.Errorf("versionMatches(%q,%q)=%v want %v", test.version, test.expression, got, test.want)
		}
	}
}

func TestApprovalTemplateSnapshotPreservesSequentialSteps(t *testing.T) {
	steps := []approvalTemplateStep{
		{Name: "技术复核", RequiredRole: "REVIEWER"},
		{Name: "生产批准", RequiredRole: "APPROVER"},
	}
	raw, err := json.Marshal(map[string]any{"name": "双人审批", "steps": steps})
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeApprovalSteps(string(raw))
	if len(decoded) != 2 || decoded[1].RequiredRole != "APPROVER" {
		t.Fatalf("unexpected snapshot steps: %#v", decoded)
	}
	policyRaw, _ := json.Marshal(map[string]any{
		"name": "管理员例外", "steps": steps, "allowSelfApproval": true,
	})
	if !approvalSnapshotAllowsSelf(string(policyRaw)) {
		t.Fatal("immutable template snapshot must preserve the administrator self-approval exception")
	}
	if approvalSnapshotAllowsSelf(string(raw)) {
		t.Fatal("self approval must remain disabled by default")
	}
}

func TestStorageKeyFromURIRejectsUncontrolledSources(t *testing.T) {
	if key, ok := storageKeyFromURI("s3://visionai/tenant/1/model.pt"); !ok || key != "tenant/1/model.pt" {
		t.Fatalf("unexpected platform storage key: %q %v", key, ok)
	}
	if _, ok := storageKeyFromURI("https://example.test/model.pt"); ok {
		t.Fatal("external HTTP artifact must not bypass controlled import validation")
	}
}
