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

	// OAuth 模式
	ModeOIDC  = "oidc"
	ModeOAuth = "oauth"

	// 用户信息获取方式
	UserinfoSourceEndpoint = "endpoint"
	UserinfoSourceToken    = "token"
	UserinfoSourceBoth     = "both"
)

// OAuthHandler OAuth 认证处理器
type OAuthHandler struct {
	config     *models.Config
	oidcConfig *OIDCConfig
	endpoints  *OAuthEndpoints
}

// OIDCConfig OIDC 发现配置
type OIDCConfig struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

// OAuthEndpoints OAuth 端点配置（统一结构）
type OAuthEndpoints struct {
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserinfoEndpoint      string
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

// getMode 获取 OAuth 模式，默认 oidc
func (h *OAuthHandler) getMode() string {
	mode := strings.ToLower(h.config.OAuth.Mode)
	if mode == ModeOAuth {
		return ModeOAuth
	}
	return ModeOIDC
}

// getEndpoints 获取 OAuth 端点
func (h *OAuthHandler) getEndpoints() (*OAuthEndpoints, error) {
	if h.endpoints != nil {
		return h.endpoints, nil
	}

	mode := h.getMode()

	if mode == ModeOAuth {
		// OAuth 模式：使用手动配置的端点
		if h.config.OAuth.AuthorizationEndpoint == "" || h.config.OAuth.TokenEndpoint == "" {
			return nil, fmt.Errorf("OAuth 模式需要配置 authorizationEndpoint 和 tokenEndpoint")
		}
		h.endpoints = &OAuthEndpoints{
			AuthorizationEndpoint: h.config.OAuth.AuthorizationEndpoint,
			TokenEndpoint:         h.config.OAuth.TokenEndpoint,
			UserinfoEndpoint:      h.config.OAuth.UserinfoEndpoint,
		}
	} else {
		// OIDC 模式：自动发现端点
		if h.oidcConfig == nil {
			if err := h.DiscoverOIDC(); err != nil {
				return nil, err
			}
		}
		h.endpoints = &OAuthEndpoints{
			AuthorizationEndpoint: h.oidcConfig.AuthorizationEndpoint,
			TokenEndpoint:         h.oidcConfig.TokenEndpoint,
			UserinfoEndpoint:      h.oidcConfig.UserinfoEndpoint,
		}
	}

	return h.endpoints, nil
}

// DiscoverOIDC 发现 OIDC 配置
func (h *OAuthHandler) DiscoverOIDC() error {
	if !h.config.OAuth.Enabled || h.config.OAuth.ProviderURL == "" {
		return fmt.Errorf("OIDC 模式需要配置 providerURL")
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

	// 获取端点
	endpoints, err := h.getEndpoints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取 OAuth 端点失败: %v", err)})
		return
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

	authURL := endpoints.AuthorizationEndpoint + "?" + params.Encode()
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

	// // 验证 state
	// state := c.Query("state")
	// stateCookie, err := c.Cookie(StateCookieName)
	// if err != nil || state != stateCookie {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 state"})
	// 	return
	// }

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
	userInfo, err := h.fetchUserInfo(c.Request.Context(), token)
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
	endpoints, err := h.getEndpoints()
	if err != nil {
		return nil, err
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", h.config.OAuth.RedirectURL)
	data.Set("client_id", h.config.OAuth.ClientID)
	data.Set("client_secret", h.config.OAuth.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoints.TokenEndpoint, strings.NewReader(data.Encode()))
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

// fetchUserInfo 根据配置获取用户信息
func (h *OAuthHandler) fetchUserInfo(ctx context.Context, token *TokenResponse) (*UserInfo, error) {
	source := strings.ToLower(h.config.OAuth.UserinfoSource)
	if source == "" {
		source = UserinfoSourceEndpoint // 默认从 endpoint 获取
	}

	var userInfo *UserInfo
	var tokenErr, endpointErr error

	// 根据 source 决定获取方式
	switch source {
	case UserinfoSourceToken:
		// 只从 token 解析
		userInfo, tokenErr = h.parseTokenClaims(token)
		if tokenErr != nil {
			return nil, fmt.Errorf("从 token 解析用户信息失败: %v", tokenErr)
		}
	case UserinfoSourceBoth:
		// 优先从 token 解析，失败则从 endpoint 获取
		userInfo, tokenErr = h.parseTokenClaims(token)
		if tokenErr != nil || userInfo.PreferredUsername == "" {
			userInfo, endpointErr = h.getUserInfoFromEndpoint(ctx, token.AccessToken)
			if endpointErr != nil {
				if tokenErr != nil {
					return nil, fmt.Errorf("从 token 和 endpoint 获取用户信息都失败: token=%v, endpoint=%v", tokenErr, endpointErr)
				}
				return nil, fmt.Errorf("从 endpoint 获取用户信息失败: %v", endpointErr)
			}
		}
	default:
		// UserinfoSourceEndpoint：从 endpoint 获取
		userInfo, endpointErr = h.getUserInfoFromEndpoint(ctx, token.AccessToken)
		if endpointErr != nil {
			return nil, fmt.Errorf("从 endpoint 获取用户信息失败: %v", endpointErr)
		}
	}

	return userInfo, nil
}

// parseTokenClaims 从 access_token 或 id_token 解析用户信息
func (h *OAuthHandler) parseTokenClaims(token *TokenResponse) (*UserInfo, error) {
	// 优先使用 id_token（OIDC），其次使用 access_token
	tokenToParse := token.IDToken
	if tokenToParse == "" {
		tokenToParse = token.AccessToken
	}

	// 尝试解析 JWT（不验证签名，只解码 payload）
	parts := strings.Split(tokenToParse, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("token 不是有效的 JWT 格式")
	}

	// 解码 payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("解码 JWT payload 失败: %v", err)
	}

	// 解析 claims
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("解析 JWT claims 失败: %v", err)
	}

	// 从 claims map 中提取用户信息
	return h.extractUserInfoFromMap(claims), nil
}

// extractUserInfoFromMap 从 map 中提取用户信息（支持字段映射配置）
func (h *OAuthHandler) extractUserInfoFromMap(data map[string]interface{}) *UserInfo {
	// 获取配置的字段名
	usernameClaim := h.config.OAuth.TokenUsernameClaim
	if usernameClaim == "" {
		usernameClaim = "preferred_username"
	}

	emailClaim := h.config.OAuth.TokenEmailClaim
	if emailClaim == "" {
		emailClaim = "email"
	}

	userInfo := &UserInfo{}

	// 提取用户名（尝试多个字段，优先使用配置的字段）
	if v, ok := data[usernameClaim].(string); ok && v != "" {
		userInfo.PreferredUsername = v
	}
	// 回退到标准字段
	if userInfo.PreferredUsername == "" {
		if v, ok := data["username"].(string); ok && v != "" {
			userInfo.PreferredUsername = v
		}
	}
	if userInfo.PreferredUsername == "" {
		if v, ok := data["preferred_username"].(string); ok && v != "" {
			userInfo.PreferredUsername = v
		}
	}
	if userInfo.PreferredUsername == "" {
		if v, ok := data["name"].(string); ok && v != "" {
			userInfo.Name = v
		}
	}

	// 提取邮箱
	if v, ok := data[emailClaim].(string); ok && v != "" {
		userInfo.Email = v
	}
	// 回退到标准字段
	if userInfo.Email == "" {
		if v, ok := data["mail"].(string); ok && v != "" {
			userInfo.Email = v
		}
	}

	// 提取 sub（用户唯一标识）
	if v, ok := data["sub"].(string); ok && v != "" {
		userInfo.Sub = v
	}
	// 回退到其他可能的 ID 字段
	if userInfo.Sub == "" {
		if v, ok := data["user_id"].(string); ok && v != "" {
			userInfo.Sub = v
		}
	}
	if userInfo.Sub == "" {
		if v, ok := data["id"].(string); ok && v != "" {
			userInfo.Sub = v
		}
	}

	return userInfo
}

// getUserInfoFromEndpoint 从 userinfo endpoint 获取用户信息
func (h *OAuthHandler) getUserInfoFromEndpoint(ctx context.Context, accessToken string) (*UserInfo, error) {
	endpoints, err := h.getEndpoints()
	if err != nil {
		return nil, err
	}

	if endpoints.UserinfoEndpoint == "" {
		return nil, fmt.Errorf("未配置 userinfo endpoint")
	}

	// 获取请求方式，默认 GET
	method := strings.ToUpper(h.config.OAuth.UserinfoMethod)
	if method == "" {
		method = "GET"
	}

	var req *http.Request

	if method == "POST" {
		// POST JSON 方式：请求体包含 client_id, access_token, scope
		reqBody := map[string]interface{}{
			"client_id":    h.config.OAuth.ClientID,
			"access_token": accessToken,
			"scope":        strings.Join(h.config.OAuth.Scopes, " "),
		}
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal userinfo request: %w", err)
		}

		req, err = http.NewRequestWithContext(ctx, "POST", endpoints.UserinfoEndpoint, strings.NewReader(string(bodyBytes)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		// GET 方式：标准 Bearer Token
		req, err = http.NewRequestWithContext(ctx, "GET", endpoints.UserinfoEndpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	// 先解析为 map，以便支持字段映射
	var rawData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawData); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo response: %w", err)
	}

	// 根据配置的字段名提取用户信息
	userInfo := h.extractUserInfoFromMap(rawData)
	return userInfo, nil
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
