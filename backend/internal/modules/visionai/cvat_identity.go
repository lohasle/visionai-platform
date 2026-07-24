package visionai

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/lohasle/nimbus-framework-go/internal/modules/system"
	"github.com/lohasle/nimbus-framework-go/internal/platform/annotation"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"gorm.io/gorm"
)

var cvatUsernameCharacters = regexp.MustCompile(`[^a-zA-Z0-9_.@+-]+`)

func credentialKey() []byte {
	sum := sha256.Sum256([]byte("visionai:cvat-credential:v1:" + config.Load().JWTSecret))
	return sum[:]
}

func encryptCVATCredential(value string) (string, error) {
	block, err := aes.NewCipher(credentialKey())
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nil, nonce, []byte(value), []byte("visionai-cvat-user"))
	return base64.RawURLEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func decryptCVATCredential(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(credentialKey())
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) <= aead.NonceSize() {
		return "", errors.New("invalid encrypted CVAT credential")
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte("visionai-cvat-user"))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func randomSecret(bytesCount int) (string, error) {
	raw := make([]byte, bytesCount)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func cvatClient() (*annotation.CVAT, error) {
	cfg := config.Load()
	return annotation.NewCVAT(cfg.CVATBaseURL, cfg.CVATPublicURL, cfg.CVATUsername, cfg.CVATPassword, cfg.CVATTimeout)
}

func cvatUsername(tenantID, userID uint64, username string) (string, error) {
	suffix, err := randomSecret(5)
	if err != nil {
		return "", err
	}
	clean := strings.Trim(cvatUsernameCharacters.ReplaceAllString(strings.ToLower(username), "_"), "_")
	if clean == "" {
		clean = "user"
	}
	if len(clean) > 32 {
		clean = clean[:32]
	}
	return fmt.Sprintf("va_t%d_u%d_%s_%s", tenantID, userID, clean, strings.ToLower(suffix)), nil
}

func (h *Handler) ensureCVATIdentity(ctx context.Context, tenantID, userID uint64) (CVATUserMapping, error) {
	var mapping CVATUserMapping
	if err := h.db.Where("tenant_id = ? AND platform_user_id = ? AND active = ?", tenantID, userID, true).First(&mapping).Error; err == nil {
		if mapping.CredentialCipher != "" {
			if _, decryptErr := decryptCVATCredential(mapping.CredentialCipher); decryptErr == nil {
				return mapping, nil
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return mapping, err
	}

	var user system.AdminUser
	if err := h.db.Where("tenant_id = ? AND id = ? AND status = ?", tenantID, userID, 0).First(&user).Error; err != nil {
		return mapping, errors.New("底座用户不存在或已停用")
	}
	provider, err := cvatClient()
	if err != nil {
		return mapping, err
	}
	username, err := cvatUsername(tenantID, userID, user.Username)
	if err != nil {
		return mapping, err
	}
	passwordSeed, err := randomSecret(32)
	if err != nil {
		return mapping, err
	}
	password := "Va1!" + passwordSeed
	email := strings.TrimSpace(user.Email)
	if email == "" {
		email = fmt.Sprintf("%s@visionai.local", username)
	}
	firstName := strings.TrimSpace(user.Nickname)
	if firstName == "" {
		firstName = user.Username
	}
	if err = provider.RegisterUser(ctx, username, email, password, firstName, "VisionAI"); err != nil {
		return mapping, err
	}
	external, err := provider.FindUser(ctx, username)
	if err != nil {
		return mapping, err
	}
	encrypted, err := encryptCVATCredential(password)
	if err != nil {
		return mapping, err
	}
	now := time.Now()
	mapping = CVATUserMapping{
		TenantID: tenantID, PlatformUserID: userID, CVATUserID: external.ID, CVATUsername: external.Username,
		CredentialCipher: encrypted, Active: true, VerifiedAt: &now,
	}
	err = h.db.Where(CVATUserMapping{TenantID: tenantID, PlatformUserID: userID}).
		Assign(map[string]any{
			"cvat_user_id": external.ID, "cvat_username": external.Username, "credential_cipher": encrypted,
			"active": true, "verified_at": now,
		}).
		FirstOrCreate(&mapping).Error
	return mapping, err
}

func (h *Handler) ensureCVATIdentities(ctx context.Context, tenantID uint64, userIDs []uint64) ([]CVATUserMapping, error) {
	seen := make(map[uint64]bool, len(userIDs))
	result := make([]CVATUserMapping, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 || seen[userID] {
			continue
		}
		seen[userID] = true
		mapping, err := h.ensureCVATIdentity(ctx, tenantID, userID)
		if err != nil {
			return nil, fmt.Errorf("同步底座用户 %d 到 CVAT 失败: %w", userID, err)
		}
		result = append(result, mapping)
	}
	if len(result) == 0 {
		return nil, errors.New("至少需要一个有效的底座用户")
	}
	return result, nil
}

func mappingIDs(rows []CVATUserMapping) []int64 {
	result := make([]int64, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.CVATUserID)
	}
	return result
}
