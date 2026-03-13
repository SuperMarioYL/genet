package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
)

const cliRefreshTokenTTL = 30 * 24 * time.Hour

type CLIAuthHandler struct {
	k8sClient *k8s.Client
	config    *models.Config
	log       *zap.Logger
}

func NewCLIAuthHandler(k8sClient *k8s.Client, config *models.Config) *CLIAuthHandler {
	return &CLIAuthHandler{
		k8sClient: k8sClient,
		config:    config,
		log:       logger.Named("cli-auth"),
	}
}

func (h *CLIAuthHandler) Start(c *gin.Context) {
	var req models.CLIAuthStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	requestID, err := auth.GenerateCLISecretForHandler()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate request id"})
		return
	}
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	record := models.CLIAuthRequestRecord{
		ID:               requestID,
		CodeChallenge:    req.CodeChallenge,
		LocalCallbackURL: req.LocalCallbackURL,
		State:            req.State,
		ExpiresAt:        expiresAt,
	}
	if err := h.k8sClient.CreateCLIAuthRequest(c.Request.Context(), record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create auth request"})
		return
	}
	c.JSON(http.StatusOK, models.CLIAuthStartResponse{
		RequestID: requestID,
		LoginURL:  "/api/cli/auth/complete?request_id=" + requestID,
		ExpiresAt: expiresAt,
	})
}

func (h *CLIAuthHandler) Complete(c *gin.Context) {
	requestID := strings.TrimSpace(c.Query("request_id"))
	if requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing request_id"})
		return
	}
	record, err := h.k8sClient.GetCLIAuthRequest(c.Request.Context(), requestID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth request not found"})
		return
	}
	if record.ExpiresAt.Before(time.Now().UTC()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth request expired"})
		return
	}
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	if strings.TrimSpace(username) == "" {
		c.Redirect(http.StatusFound, "/api/auth/login?return_to="+url.QueryEscape(c.Request.URL.RequestURI()))
		return
	}
	code, err := auth.GenerateCLISecretForHandler()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate auth code"})
		return
	}
	record.Username = username
	record.Email = email
	record.AuthCodeHash = k8s.HashCLISecretForTest(code)
	if err := h.k8sClient.UpdateCLIAuthRequest(c.Request.Context(), *record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update auth request"})
		return
	}
	redirectURL, err := url.Parse(record.LocalCallbackURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid callback url"})
		return
	}
	query := redirectURL.Query()
	query.Set("code", code)
	query.Set("state", record.State)
	redirectURL.RawQuery = query.Encode()
	c.Redirect(http.StatusFound, redirectURL.String())
}

func (h *CLIAuthHandler) Exchange(c *gin.Context) {
	var req models.CLIAuthExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	record, err := h.k8sClient.GetCLIAuthRequest(c.Request.Context(), req.RequestID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "auth request not found"})
		return
	}
	if record.ExpiresAt.Before(time.Now().UTC()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth request expired"})
		return
	}
	if err := auth.ValidatePKCEChallengeForHandler(req.CodeVerifier, record.CodeChallenge); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid code verifier"})
		return
	}
	record, err = h.k8sClient.ConsumeCLIAuthRequest(c.Request.Context(), req.RequestID, req.Code, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid auth code"})
		return
	}
	sessionID, err := auth.GenerateCLISecretForHandler()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate session id"})
		return
	}
	accessToken, err := auth.CreateCLIAccessTokenForHandler(h.config, record.Username, record.Email, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create access token"})
		return
	}
	refreshToken, err := auth.GenerateCLISecretForHandler()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate refresh token"})
		return
	}
	expiresAt := time.Now().UTC().Add(auth.DefaultCLIAccessTokenTTLForHandler())
	if err := h.k8sClient.CreateCLIRefreshSession(c.Request.Context(), models.CLIRefreshSessionRecord{
		ID:        sessionID,
		TokenHash: k8s.HashCLISecretForTest(refreshToken),
		Username:  record.Username,
		Email:     record.Email,
		UserAgent: c.Request.UserAgent(),
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(cliRefreshTokenTTL),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist refresh session"})
		return
	}
	c.JSON(http.StatusOK, models.CLIAuthTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		Username:     record.Username,
		Email:        record.Email,
	})
}

func (h *CLIAuthHandler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	record, ok, err := h.k8sClient.FindCLIRefreshSessionByPlaintext(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to lookup refresh session"})
		return
	}
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}
	nextRefresh, err := auth.GenerateCLISecretForHandler()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate refresh token"})
		return
	}
	rotated, err := h.k8sClient.RotateCLIRefreshSession(c.Request.Context(), record.ID, nextRefresh, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate refresh session"})
		return
	}
	accessToken, err := auth.CreateCLIAccessTokenForHandler(h.config, rotated.Username, rotated.Email, rotated.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create access token"})
		return
	}
	c.JSON(http.StatusOK, models.CLIAuthTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: nextRefresh,
		ExpiresAt:    time.Now().UTC().Add(auth.DefaultCLIAccessTokenTTLForHandler()),
		Username:     rotated.Username,
		Email:        rotated.Email,
	})
}

func (h *CLIAuthHandler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	record, ok, err := h.k8sClient.FindCLIRefreshSessionByPlaintext(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to lookup refresh session"})
		return
	}
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}
	if err := h.k8sClient.RevokeCLIRefreshSession(c.Request.Context(), record.ID, time.Now().UTC()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke refresh session"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}
