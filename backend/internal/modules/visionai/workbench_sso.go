package visionai

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lohasle/nimbus-framework-go/internal/platform/config"
	"github.com/lohasle/nimbus-framework-go/internal/platform/httpx"
	"gorm.io/gorm"
)

const (
	workbenchTicketTTL       = 90 * time.Second
	fiftyOneSessionTTL       = 8 * time.Hour
	fiftyOneSessionCookie    = "visionai_fiftyone_session"
	workbenchTicketQueryName = "ticket"
)

type workbenchSessionClaims struct {
	Provider  string `json:"provider"`
	TenantID  uint64 `json:"tenantId"`
	ProjectID uint64 `json:"projectId"`
	UserID    uint64 `json:"userId"`
	ExpiresAt int64  `json:"expiresAt"`
}

func ticketHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func configuredWorkbenchBase(provider string) string {
	cfg := config.Load()
	base := cfg.FiftyOnePublicURL
	if provider == "CVAT" {
		base = cfg.CVATPublicURL
	}
	return strings.TrimRight(base, "/")
}

func requestHostname(c *gin.Context) string {
	parsed, err := url.Parse("//" + c.Request.Host)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func workbenchBaseForRequest(c *gin.Context, provider string) (string, error) {
	baseURL, err := url.Parse(configuredWorkbenchBase(provider))
	if err != nil || baseURL.Hostname() == "" {
		return "", errors.New("工作台公开地址配置无效")
	}
	requestHost := requestHostname(c)
	if requestHost == "" {
		return "", errors.New("工作台请求主机无效")
	}
	if origin := strings.TrimSpace(c.GetHeader("Origin")); origin != "" {
		originURL, originErr := url.Parse(origin)
		if originErr != nil || !strings.EqualFold(originURL.Hostname(), requestHost) {
			return "", errors.New("工作台请求来源与访问主机不一致")
		}
	}
	if port := baseURL.Port(); port != "" {
		baseURL.Host = net.JoinHostPort(requestHost, port)
	} else {
		baseURL.Host = requestHost
	}
	return strings.TrimRight(baseURL.String(), "/"), nil
}

func workbenchTargetForRequest(c *gin.Context, provider, target string) (string, error) {
	base, err := workbenchBaseForRequest(c, provider)
	if err != nil {
		return "", err
	}
	baseURL, _ := url.Parse(base)
	targetURL, targetErr := url.Parse(target)
	if targetErr != nil || targetURL.Path == "" && targetURL.RawQuery == "" {
		return "", errors.New("工作台目标地址无效")
	}
	targetURL.Scheme, targetURL.Host = baseURL.Scheme, baseURL.Host
	return targetURL.String(), nil
}

func allowedWorkbenchRedirect(c *gin.Context, provider, target string) bool {
	base := configuredWorkbenchBase(provider)
	baseURL, baseErr := url.Parse(strings.TrimRight(base, "/"))
	targetURL, targetErr := url.Parse(target)
	return baseErr == nil && targetErr == nil &&
		(baseURL.Scheme == "http" || baseURL.Scheme == "https") &&
		targetURL.Scheme == baseURL.Scheme &&
		targetURL.Port() == baseURL.Port() &&
		strings.EqualFold(targetURL.Hostname(), requestHostname(c))
}

func (h *Handler) createWorkbenchLaunch(c *gin.Context, provider string, project Project, userID uint64, resourceType string, resourceID uint64, target string) (string, error) {
	provider = strings.ToUpper(strings.TrimSpace(provider))
	target, err := workbenchTargetForRequest(c, provider, target)
	if err != nil || !allowedWorkbenchRedirect(c, provider, target) {
		return "", errors.New("工作台重定向地址不受信任")
	}
	token, err := randomSecret(32)
	if err != nil {
		return "", err
	}
	now := time.Now()
	ticket := WorkbenchTicket{
		TokenHash: ticketHash(token), Provider: provider, TenantID: project.TenantID, ProjectID: project.ID,
		UserID: userID, ResourceType: resourceType, ResourceID: resourceID, RedirectURL: target,
		ExpiresAt: now.Add(workbenchTicketTTL),
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		_ = tx.Where("expires_at < ?", now.Add(-24*time.Hour)).Delete(&WorkbenchTicket{}).Error
		return tx.Create(&ticket).Error
	})
	if err != nil {
		return "", err
	}
	path := "/admin-api/ai-platform/workbench-sso/fiftyone"
	if provider == "CVAT" {
		path = "/admin-api/ai-platform/workbench-sso/cvat"
	}
	targetURL, _ := url.Parse(target)
	return targetURL.Scheme + "://" + targetURL.Host + path + "?" + workbenchTicketQueryName + "=" + url.QueryEscape(token), nil
}

func (h *Handler) consumeWorkbenchTicket(token, provider string) (WorkbenchTicket, error) {
	var ticket WorkbenchTicket
	if strings.TrimSpace(token) == "" {
		return ticket, errors.New("缺少工作台票据")
	}
	now := time.Now()
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("token_hash = ? AND provider = ?", ticketHash(token), strings.ToUpper(provider)).First(&ticket).Error; err != nil {
			return err
		}
		if ticket.ConsumedAt != nil {
			return errors.New("工作台票据已使用")
		}
		if !ticket.ExpiresAt.After(now) {
			return errors.New("工作台票据已过期")
		}
		result := tx.Model(&WorkbenchTicket{}).
			Where("id = ? AND consumed_at IS NULL AND expires_at > ?", ticket.ID, now).
			Update("consumed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("工作台票据已失效")
		}
		ticket.ConsumedAt = &now
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ticket, errors.New("工作台票据无效")
	}
	return ticket, err
}

func signWorkbenchSession(claims workbenchSessionClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte("visionai:workbench-session:v1:"+config.Load().JWTSecret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func verifyWorkbenchSession(value, provider string) (workbenchSessionClaims, error) {
	var claims workbenchSessionClaims
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return claims, errors.New("invalid workbench session")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, err
	}
	mac := hmac.New(sha256.New, []byte("visionai:workbench-session:v1:"+config.Load().JWTSecret))
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return claims, errors.New("invalid workbench session signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, err
	}
	if err = json.Unmarshal(payload, &claims); err != nil {
		return claims, err
	}
	if claims.Provider != strings.ToUpper(provider) || claims.ExpiresAt <= time.Now().Unix() || claims.UserID == 0 || claims.TenantID == 0 {
		return claims, errors.New("expired or mismatched workbench session")
	}
	return claims, nil
}

// WorkbenchSSOCVAT godoc
// @Summary Exchange a one-time VisionAI ticket for a personal CVAT session
// @Tags VisionAI Workbench
// @Produce html
// @Router /ai-platform/workbench-sso/cvat [get]
func (h *Handler) WorkbenchSSOCVAT(c *gin.Context) {
	ticket, err := h.consumeWorkbenchTicket(c.Query(workbenchTicketQueryName), "CVAT")
	if err != nil || !allowedWorkbenchRedirect(c, "CVAT", ticket.RedirectURL) {
		c.String(http.StatusUnauthorized, "CVAT 工作台链接已失效，请返回 VisionAI 重新打开。")
		return
	}
	var mapping CVATUserMapping
	if h.db.Where("tenant_id = ? AND platform_user_id = ? AND active = ?", ticket.TenantID, ticket.UserID, true).First(&mapping).Error != nil {
		c.String(http.StatusUnauthorized, "CVAT 个人身份不存在，请返回 VisionAI 重新同步。")
		return
	}
	password, err := decryptCVATCredential(mapping.CredentialCipher)
	if err != nil {
		c.String(http.StatusUnauthorized, "CVAT 个人身份凭据已失效，请联系管理员重新同步。")
		return
	}
	provider, err := cvatClient()
	if err != nil {
		c.String(http.StatusServiceUnavailable, "CVAT 服务配置不可用。")
		return
	}
	cookies, err := provider.Login(c.Request.Context(), mapping.CVATUsername, password)
	if err != nil {
		c.String(http.StatusBadGateway, "CVAT 个人会话建立失败，请返回 VisionAI 重试。")
		return
	}
	c.Header("Cache-Control", "no-store")
	for _, cookie := range cookies {
		c.Writer.Header().Add("Set-Cookie", cookie)
	}
	c.Redirect(http.StatusSeeOther, ticket.RedirectURL)
}

// WorkbenchSSOFiftyOne godoc
// @Summary Exchange a one-time VisionAI ticket for a protected FiftyOne session
// @Tags VisionAI Workbench
// @Produce html
// @Router /ai-platform/workbench-sso/fiftyone [get]
func (h *Handler) WorkbenchSSOFiftyOne(c *gin.Context) {
	ticket, err := h.consumeWorkbenchTicket(c.Query(workbenchTicketQueryName), "FIFTYONE")
	if err != nil || !allowedWorkbenchRedirect(c, "FIFTYONE", ticket.RedirectURL) {
		c.String(http.StatusUnauthorized, "FiftyOne 工作台链接已失效，请返回 VisionAI 重新打开。")
		return
	}
	session, err := signWorkbenchSession(workbenchSessionClaims{
		Provider: "FIFTYONE", TenantID: ticket.TenantID, ProjectID: ticket.ProjectID, UserID: ticket.UserID,
		ExpiresAt: time.Now().Add(fiftyOneSessionTTL).Unix(),
	})
	if err != nil {
		c.String(http.StatusInternalServerError, "FiftyOne 工作台会话创建失败。")
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(fiftyOneSessionCookie, session, int(fiftyOneSessionTTL.Seconds()), "/", "", c.Request.URL.Scheme == "https", true)
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusSeeOther, ticket.RedirectURL)
}

// WorkbenchSSOFiftyOneValidate godoc
// @Summary Validate a FiftyOne gateway session
// @Tags VisionAI Workbench
// @Router /ai-platform/workbench-sso/fiftyone/validate [get]
func (h *Handler) WorkbenchSSOFiftyOneValidate(c *gin.Context) {
	session, err := c.Cookie(fiftyOneSessionCookie)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	claims, err := verifyWorkbenchSession(session, "FIFTYONE")
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	c.Header("X-VisionAI-Tenant-ID", strconv.FormatUint(claims.TenantID, 10))
	c.Header("X-VisionAI-User-ID", strconv.FormatUint(claims.UserID, 10))
	c.Status(http.StatusNoContent)
}

func workbenchLaunchError(c *gin.Context, message string) {
	httpx.Fail(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, message)
}
