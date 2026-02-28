package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
)

// AdminHandler 管理接口处理器
type AdminHandler struct {
	config    *models.Config
	k8sClient *k8s.Client
	log       *zap.Logger
}

// AdminMeResponse 管理员身份检查响应
type AdminMeResponse struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	IsAdmin  bool   `json:"isAdmin"`
}

// AdminAPIKeyItem 管理页显示的 API Key 条目
type AdminAPIKeyItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	OwnerUser  string     `json:"ownerUser"`
	Scope      string     `json:"scope"`
	Enabled    bool       `json:"enabled"`
	KeyPreview string     `json:"keyPreview,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	CreatedBy  string     `json:"createdBy"`
}

// AdminAPIKeyListResponse API Key 列表响应
type AdminAPIKeyListResponse struct {
	Items []AdminAPIKeyItem `json:"items"`
}

// CreateAdminAPIKeyRequest 创建 API Key 请求
type CreateAdminAPIKeyRequest struct {
	Name      string     `json:"name" binding:"required"`
	OwnerUser string     `json:"ownerUser" binding:"required"`
	Scope     string     `json:"scope,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// UpdateAdminAPIKeyRequest 更新 API Key 请求
type UpdateAdminAPIKeyRequest struct {
	Name      *string    `json:"name,omitempty"`
	OwnerUser *string    `json:"ownerUser,omitempty"`
	Scope     *string    `json:"scope,omitempty"`
	Enabled   *bool      `json:"enabled,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// NewAdminHandler 创建管理处理器
func NewAdminHandler(config *models.Config, k8sClient *k8s.Client) *AdminHandler {
	return &AdminHandler{
		config:    config,
		k8sClient: k8sClient,
		log:       logger.Named("admin"),
	}
}

// GetMe 返回当前用户的管理员状态。
func (h *AdminHandler) GetMe(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)

	c.JSON(http.StatusOK, AdminMeResponse{
		Username: username,
		Email:    email,
		IsAdmin:  auth.IsAdmin(h.config, username, email),
	})
}

// ListAPIKeys 列出所有管理 API Key。
func (h *AdminHandler) ListAPIKeys(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	records, err := h.k8sClient.ListOpenAPIKeys(c.Request.Context())
	if err != nil {
		h.log.Error("Failed to list OpenAPI keys", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list api keys"})
		return
	}

	items := make([]AdminAPIKeyItem, 0, len(records))
	for _, rec := range records {
		items = append(items, toAdminAPIKeyItem(rec))
	}

	c.JSON(http.StatusOK, AdminAPIKeyListResponse{Items: items})
}

// CreateAPIKey 创建一个新的管理 API Key。
func (h *AdminHandler) CreateAPIKey(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	var req CreateAdminAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.OwnerUser = strings.TrimSpace(req.OwnerUser)
	scope := normalizeAPIKeyScope(req.Scope)
	if req.Name == "" || req.OwnerUser == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and ownerUser are required"})
		return
	}
	if !isValidAPIKeyScope(scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope, must be read or write"})
		return
	}

	plaintextKey, err := generateOpenAPIPlaintextKey()
	if err != nil {
		h.log.Error("Failed to generate plaintext key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate api key"})
		return
	}

	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	createdBy := strings.TrimSpace(username)
	if createdBy == "" {
		createdBy = strings.TrimSpace(email)
	}

	now := time.Now().UTC()
	rec := models.APIKeyRecord{
		ID:         generateOpenAPIRecordID(now),
		Name:       req.Name,
		OwnerUser:  req.OwnerUser,
		Scope:      scope,
		Enabled:    true,
		KeyHash:    k8s.HashOpenAPIKey(plaintextKey),
		KeyPreview: buildOpenAPIKeyPreview(plaintextKey),
		ExpiresAt:  req.ExpiresAt,
		CreatedAt:  now,
		UpdatedAt:  now,
		CreatedBy:  createdBy,
	}

	if err := h.k8sClient.CreateOpenAPIKey(c.Request.Context(), rec); err != nil {
		h.log.Error("Failed to create api key record", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create api key"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"item":         toAdminAPIKeyItem(rec),
		"plaintextKey": plaintextKey,
	})
}

// UpdateAPIKey 更新 API Key 元数据。
func (h *AdminHandler) UpdateAPIKey(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key id"})
		return
	}

	var req UpdateAdminAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	records, err := h.k8sClient.ListOpenAPIKeys(c.Request.Context())
	if err != nil {
		h.log.Error("Failed to list keys before update", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update api key"})
		return
	}

	var target *models.APIKeyRecord
	for i := range records {
		if records[i].ID == id {
			target = &records[i]
			break
		}
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
		return
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty"})
			return
		}
		target.Name = name
	}
	if req.OwnerUser != nil {
		owner := strings.TrimSpace(*req.OwnerUser)
		if owner == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ownerUser cannot be empty"})
			return
		}
		target.OwnerUser = owner
	}
	if req.Scope != nil {
		scope := normalizeAPIKeyScope(*req.Scope)
		if !isValidAPIKeyScope(scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope, must be read or write"})
			return
		}
		target.Scope = scope
	}
	if req.Enabled != nil {
		target.Enabled = *req.Enabled
	}
	if req.ExpiresAt != nil {
		target.ExpiresAt = req.ExpiresAt
	}
	target.UpdatedAt = time.Now().UTC()

	if err := h.k8sClient.UpdateOpenAPIKey(c.Request.Context(), *target); err != nil {
		if errors.Is(err, k8s.ErrOpenAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
			return
		}
		h.log.Error("Failed to update api key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update api key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"item": toAdminAPIKeyItem(*target)})
}

// DeleteAPIKey 删除 API Key。
func (h *AdminHandler) DeleteAPIKey(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key id"})
		return
	}

	if err := h.k8sClient.DeleteOpenAPIKey(c.Request.Context(), id); err != nil {
		if errors.Is(err, k8s.ErrOpenAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
			return
		}
		h.log.Error("Failed to delete api key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete api key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func normalizeAPIKeyScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return models.APIKeyScopeRead
	}
	return scope
}

func isValidAPIKeyScope(scope string) bool {
	return scope == models.APIKeyScopeRead || scope == models.APIKeyScopeWrite
}

func toAdminAPIKeyItem(rec models.APIKeyRecord) AdminAPIKeyItem {
	return AdminAPIKeyItem{
		ID:         rec.ID,
		Name:       rec.Name,
		OwnerUser:  rec.OwnerUser,
		Scope:      rec.Scope,
		Enabled:    rec.Enabled,
		KeyPreview: rec.KeyPreview,
		ExpiresAt:  rec.ExpiresAt,
		CreatedAt:  rec.CreatedAt,
		UpdatedAt:  rec.UpdatedAt,
		CreatedBy:  rec.CreatedBy,
	}
}

func generateOpenAPIPlaintextKey() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "gk_" + hex.EncodeToString(buf), nil
}

func generateOpenAPIRecordID(now time.Time) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("ak_%d", now.UnixNano())
	}
	return fmt.Sprintf("ak_%d_%s", now.UnixNano(), hex.EncodeToString(buf))
}

func buildOpenAPIKeyPreview(plaintext string) string {
	if len(plaintext) <= 12 {
		return plaintext
	}
	return plaintext[:12] + "..."
}
