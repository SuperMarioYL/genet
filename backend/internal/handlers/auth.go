package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/models"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	config *models.Config
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(config *models.Config) *AuthHandler {
	return &AuthHandler{
		config: config,
	}
}

// AuthStatusResponse 认证状态响应
type AuthStatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
	Email         string `json:"email,omitempty"`
	OAuthEnabled  bool   `json:"oauthEnabled"`
	LoginURL      string `json:"loginURL,omitempty"`
}

// GetAuthStatus 获取认证状态
func (h *AuthHandler) GetAuthStatus(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)

	// 从 context 获取认证状态（由中间件设置）
	authenticated, _ := c.Get("authenticated")
	isAuthenticated, _ := authenticated.(bool)

	// 如果 OAuth 未启用，使用默认用户也算认证
	if !h.config.OAuth.Enabled && username != "" {
		isAuthenticated = true
	}

	response := AuthStatusResponse{
		Authenticated: isAuthenticated,
		Username:      username,
		Email:         email,
		OAuthEnabled:  h.config.OAuth.Enabled,
	}

	// 如果 OAuth 已启用且未认证，返回登录 URL
	if h.config.OAuth.Enabled && !isAuthenticated {
		response.LoginURL = "/api/auth/login"
	}

	c.JSON(http.StatusOK, response)
}

