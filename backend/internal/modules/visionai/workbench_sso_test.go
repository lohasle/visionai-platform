package visionai

import (
	"strings"
	"testing"
	"time"
)

func TestCVATCredentialEncryption(t *testing.T) {
	encrypted, err := encryptCVATCredential("unique-personal-secret")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "unique-personal-secret" || strings.Contains(encrypted, "personal") {
		t.Fatal("credential was not encrypted")
	}
	decrypted, err := decryptCVATCredential(encrypted)
	if err != nil || decrypted != "unique-personal-secret" {
		t.Fatalf("decrypt = %q, %v", decrypted, err)
	}
	if _, err = decryptCVATCredential(encrypted[:len(encrypted)-2] + "xx"); err == nil {
		t.Fatal("tampered credential must be rejected")
	}
}

func TestWorkbenchSessionSigning(t *testing.T) {
	claims := workbenchSessionClaims{
		Provider: "FIFTYONE", TenantID: 1, ProjectID: 12, UserID: 3,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	session, err := signWorkbenchSession(claims)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := verifyWorkbenchSession(session, "FIFTYONE")
	if err != nil || verified.UserID != claims.UserID || verified.ProjectID != claims.ProjectID {
		t.Fatalf("verified = %+v, %v", verified, err)
	}
	if _, err = verifyWorkbenchSession(session+"x", "FIFTYONE"); err == nil {
		t.Fatal("tampered session must be rejected")
	}
	expired, _ := signWorkbenchSession(workbenchSessionClaims{
		Provider: "FIFTYONE", TenantID: 1, UserID: 3, ExpiresAt: time.Now().Add(-time.Second).Unix(),
	})
	if _, err = verifyWorkbenchSession(expired, "FIFTYONE"); err == nil {
		t.Fatal("expired session must be rejected")
	}
}
