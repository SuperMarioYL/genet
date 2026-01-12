package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/uc-package/genet/internal/models"
)

const (
	// Cookie 名称
	SessionCookieName = "genet_session"
	StateCookieName   = "genet_oauth_state"

	// Session 有效期
	SessionDuration = 24 * time.Hour
)

// OAuthHandler OAuth 认证处理器
type OAuthHandler struct {
	config     *models.Config
	oidcConfig *OIDCConfig
}

// OIDCConfig OIDC 发现配置
type OIDCConfig struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

// TokenResponse OAuth Token 响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// UserInfo 用户信息
type UserInfo struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
}

// SessionClaims JWT Session Claims
type SessionClaims struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

// NewOAuthHandler 创建 OAuth 处理器
func NewOAuthHandler(config *models.Config) *OAuthHandler {
	return &OAuthHandler{
		config: config,
	}
}

// DiscoverOIDC 发现 OIDC 配置
func (h *OAuthHandler) DiscoverOIDC() error {
	if !h.config.OAuth.Enabled || h.config.OAuth.ProviderURL == "" {
		return nil
	}

	discoveryURL := strings.TrimSuffix(h.config.OAuth.ProviderURL, "/") + "/.well-known/openid-configuration"
	resp, err := http.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("failed to fetch OIDC discovery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OIDC discovery returned status %d", resp.StatusCode)
	}

	var oidcConfig OIDCConfig
	if err := json.NewDecoder(resp.Body).Decode(&oidcConfig); err != nil {
		return fmt.Errorf("failed to decode OIDC config: %w", err)
	}

	h.oidcConfig = &oidcConfig
	return nil
}

// generateState 生成随机 state
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Login 处理登录请求，重定向到 OAuth Provider
func (h *OAuthHandler) Login(c *gin.Context) {
	if !h.config.OAuth.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OAuth 未启用"})
		return
	}

	// 确保已发现 OIDC 配置
	if h.oidcConfig == nil {
		if err := h.DiscoverOIDC(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("OIDC 发现失败: %v", err)})
			return
		}
	}

	// 生成 state
	state, err := generateState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成 state 失败"})
		return
	}

	// 设置 state cookie
	c.SetCookie(
		StateCookieName,
		state,
		300, // 5 分钟
		"/",
		h.config.OAuth.CookieDomain,
		h.config.OAuth.CookieSecure,
		true, // HttpOnly
	)

	// 构建授权 URL
	params := url.Values{}
	params.Set("client_id", h.config.OAuth.ClientID)
	params.Set("redirect_uri", h.config.OAuth.RedirectURL)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(h.config.OAuth.Scopes, " "))
	params.Set("state", state)

	authURL := h.oidcConfig.AuthorizationEndpoint + "?" + params.Encode()
	c.Redirect(http.StatusFound, authURL)
}

// Callback 处理 OAuth 回调
func (h *OAuthHandler) Callback(c *gin.Context) {
	if !h.config.OAuth.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OAuth 未启用"})
		return
	}

	// 检查错误
	if errParam := c.Query("error"); errParam != "" {
		errDesc := c.Query("error_description")
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("OAuth 错误: %s - %s", errParam, errDesc)})
		return
	}

	// 验证 state
	state := c.Query("state")
	stateCookie, err := c.Cookie(StateCookieName)
	if err != nil || state != stateCookie {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 state"})
		return
	}

	// 清除 state cookie
	c.SetCookie(StateCookieName, "", -1, "/", h.config.OAuth.CookieDomain, h.config.OAuth.CookieSecure, true)

	// 获取 authorization code
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 authorization code"})
		return
	}

	// 用 code 换取 token
	token, err := h.exchangeToken(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Token 交换失败: %v", err)})
		return
	}

	// 获取用户信息
	userInfo, err := h.getUserInfo(c.Request.Context(), token.AccessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取用户信息失败: %v", err)})
		return
	}

	// 确定用户名
	username := userInfo.PreferredUsername
	if username == "" {
		username = userInfo.Name
	}
	if username == "" {
		username = userInfo.Sub
	}

	// 生成 session JWT
	sessionToken, err := h.createSessionToken(username, userInfo.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 session 失败"})
		return
	}

	// 设置 session cookie
	c.SetCookie(
		SessionCookieName,
		sessionToken,
		int(SessionDuration.Seconds()),
		"/",
		h.config.OAuth.CookieDomain,
		h.config.OAuth.CookieSecure,
		true, // HttpOnly
	)

	// 重定向到前端
	frontendURL := h.config.OAuth.FrontendURL
	if frontendURL == "" {
		frontendURL = "/"
	}
	c.Redirect(http.StatusFound, frontendURL)
}

// exchangeToken 用 code 换取 token
func (h *OAuthHandler) exchangeToken(ctx context.Context, code string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", h.config.OAuth.RedirectURL)
	data.Set("client_id", h.config.OAuth.ClientID)
	data.Set("client_secret", h.config.OAuth.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", h.oidcConfig.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var token TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	return &token, nil
}

// getUserInfo 获取用户信息
func (h *OAuthHandler) getUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.oidcConfig.UserinfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

// createSessionToken 创建 session JWT
func (h *OAuthHandler) createSessionToken(username, email string) (string, error) {
	claims := SessionClaims{
		Username: username,
		Email:    email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(SessionDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "genet",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.config.OAuth.JWTSecret))
}

// Logout 处理登出请求
func (h *OAuthHandler) Logout(c *gin.Context) {
	// 清除 session cookie
	c.SetCookie(
		SessionCookieName,
		"",
		-1,
		"/",
		h.config.OAuth.CookieDomain,
		h.config.OAuth.CookieSecure,
		true,
	)

	// 返回成功或重定向
	if c.Query("redirect") != "" {
		c.Redirect(http.StatusFound, c.Query("redirect"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已登出"})
}

// ValidateSession 验证 session token
func (h *OAuthHandler) ValidateSession(tokenString string) (*SessionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &SessionClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(h.config.OAuth.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*SessionClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

