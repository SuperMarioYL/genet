package oidc

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
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
)

const (
	// Cookie 名称
	OIDCStateCookieName = "genet_oidc_state"

	// Token 有效期
	IDTokenDuration      = 1 * time.Hour
	AccessTokenDuration  = 1 * time.Hour
	RefreshTokenDuration = 24 * time.Hour

	// 授权码有效期
	AuthCodeDuration = 5 * time.Minute
)

// Provider OIDC Provider
type Provider struct {
	config     *models.Config
	keyManager *KeyManager
	k8sClient  *k8s.Client
	log        *zap.Logger

	// 授权码存储（生产环境应使用 Redis 等持久化存储）
	authCodes     map[string]*AuthCode
	authCodeMutex sync.RWMutex

	// Refresh Token 存储
	refreshTokens     map[string]*RefreshTokenData
	refreshTokenMutex sync.RWMutex
}

// AuthCode 授权码数据
type AuthCode struct {
	Code        string
	ClientID    string
	RedirectURI string
	Username    string
	Email       string
	Scopes      []string
	ExpiresAt   time.Time
	Nonce       string
}

// RefreshTokenData Refresh Token 数据
type RefreshTokenData struct {
	Token     string
	Username  string
	Email     string
	ClientID  string
	Scopes    []string
	ExpiresAt time.Time
}

// IDTokenClaims ID Token 的 Claims
type IDTokenClaims struct {
	jwt.RegisteredClaims
	Nonce             string   `json:"nonce,omitempty"`
	Email             string   `json:"email,omitempty"`
	EmailVerified     bool     `json:"email_verified,omitempty"`
	PreferredUsername string   `json:"preferred_username,omitempty"`
	Name              string   `json:"name,omitempty"`
	Groups            []string `json:"groups,omitempty"`
}

// TokenResponse Token 响应
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// UserInfo 用户信息响应
type UserInfoResponse struct {
	Sub               string   `json:"sub"`
	Name              string   `json:"name,omitempty"`
	PreferredUsername string   `json:"preferred_username,omitempty"`
	Email             string   `json:"email,omitempty"`
	EmailVerified     bool     `json:"email_verified,omitempty"`
	Groups            []string `json:"groups,omitempty"`
}

// NewProvider 创建 OIDC Provider
func NewProvider(config *models.Config, k8sClient *k8s.Client) (*Provider, error) {
	log := logger.Named("oidc")

	p := &Provider{
		config:        config,
		keyManager:    NewKeyManager(),
		k8sClient:     k8sClient,
		log:           log,
		authCodes:     make(map[string]*AuthCode),
		refreshTokens: make(map[string]*RefreshTokenData),
	}

	// 初始化密钥
	if config.OIDCProvider.RSAPrivateKey != "" {
		// 从配置加载密钥
		if err := p.keyManager.LoadKeys(config.OIDCProvider.RSAPrivateKey, ""); err != nil {
			log.Error("Failed to load RSA private key", zap.Error(err))
			return nil, fmt.Errorf("加载 RSA 密钥失败: %w", err)
		}
		log.Info("RSA private key loaded from config")
	} else {
		// 自动生成密钥
		if err := p.keyManager.GenerateKeys(); err != nil {
			log.Error("Failed to generate RSA keys", zap.Error(err))
			return nil, fmt.Errorf("生成 RSA 密钥失败: %w", err)
		}
		log.Info("RSA keys auto-generated")
		log.Warn("Production environment should use fixed RSA keys, otherwise tokens will be invalidated after restart")
	}

	// 启动清理过期数据的 goroutine
	go p.cleanupExpiredData()

	log.Info("OIDC Provider initialized",
		zap.String("issuer", config.OIDCProvider.IssuerURL),
		zap.String("k8sClientID", config.OIDCProvider.KubernetesClientID))

	return p, nil
}

// cleanupExpiredData 定期清理过期的授权码和 Refresh Token
func (p *Provider) cleanupExpiredData() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		now := time.Now()
		cleanedCodes := 0
		cleanedTokens := 0

		// 清理授权码
		p.authCodeMutex.Lock()
		for code, data := range p.authCodes {
			if now.After(data.ExpiresAt) {
				delete(p.authCodes, code)
				cleanedCodes++
			}
		}
		p.authCodeMutex.Unlock()

		// 清理 Refresh Token
		p.refreshTokenMutex.Lock()
		for token, data := range p.refreshTokens {
			if now.After(data.ExpiresAt) {
				delete(p.refreshTokens, token)
				cleanedTokens++
			}
		}
		p.refreshTokenMutex.Unlock()

		if cleanedCodes > 0 || cleanedTokens > 0 {
			p.log.Debug("Cleaned up expired data",
				zap.Int("authCodes", cleanedCodes),
				zap.Int("refreshTokens", cleanedTokens))
		}
	}
}

// GetIssuerURL 获取 Issuer URL
func (p *Provider) GetIssuerURL() string {
	return strings.TrimSuffix(p.config.OIDCProvider.IssuerURL, "/")
}

// RegisterRoutes 注册 OIDC 路由
func (p *Provider) RegisterRoutes(r *gin.Engine) {
	// OIDC Discovery
	r.GET("/.well-known/openid-configuration", p.Discovery)

	// OIDC 端点
	oidc := r.Group("/oidc")
	{
		oidc.GET("/authorize", p.Authorize)
		oidc.POST("/token", p.Token)
		oidc.GET("/userinfo", p.UserInfo)
		oidc.POST("/userinfo", p.UserInfo)
		oidc.GET("/jwks", p.JWKS)
	}

	p.log.Info("OIDC routes registered",
		zap.String("discovery", "/.well-known/openid-configuration"),
		zap.String("authorize", "/oidc/authorize"),
		zap.String("token", "/oidc/token"),
		zap.String("userinfo", "/oidc/userinfo"),
		zap.String("jwks", "/oidc/jwks"))
}

// Discovery OIDC 发现端点
func (p *Provider) Discovery(c *gin.Context) {
	issuer := p.GetIssuerURL()

	p.log.Debug("OIDC discovery requested",
		zap.String("clientIP", c.ClientIP()))

	discovery := map[string]interface{}{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oidc/authorize",
		"token_endpoint":                        issuer + "/oidc/token",
		"userinfo_endpoint":                     issuer + "/oidc/userinfo",
		"jwks_uri":                              issuer + "/oidc/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "groups"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"claims_supported": []string{
			"sub", "iss", "aud", "exp", "iat", "nonce",
			"name", "preferred_username", "email", "email_verified", "groups",
		},
		"grant_types_supported": []string{"authorization_code", "refresh_token"},
	}

	c.JSON(http.StatusOK, discovery)
}

// JWKS JSON Web Key Set 端点
func (p *Provider) JWKS(c *gin.Context) {
	p.log.Debug("JWKS requested", zap.String("clientIP", c.ClientIP()))
	c.JSON(http.StatusOK, p.keyManager.GetJWKS())
}

// Authorize 授权端点
func (p *Provider) Authorize(c *gin.Context) {
	clientID := c.Query("client_id")
	redirectURI := c.Query("redirect_uri")
	responseType := c.Query("response_type")
	scope := c.Query("scope")
	state := c.Query("state")
	nonce := c.Query("nonce")

	p.log.Info("Authorization request received",
		zap.String("clientID", clientID),
		zap.String("redirectURI", redirectURI),
		zap.String("responseType", responseType),
		zap.String("scope", scope),
		zap.String("clientIP", c.ClientIP()))

	// 验证参数
	if clientID == "" || redirectURI == "" || responseType != "code" {
		p.log.Warn("Invalid authorization request",
			zap.String("clientID", clientID),
			zap.String("redirectURI", redirectURI),
			zap.String("responseType", responseType))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "缺少必要参数或 response_type 不支持",
		})
		return
	}

	// 验证 client_id（这里简化处理，生产环境应该有客户端注册机制）
	if clientID != p.config.OIDCProvider.KubernetesClientID && clientID != p.config.OAuth.ClientID {
		p.log.Warn("Unknown client_id", zap.String("clientID", clientID))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_client",
			"error_description": "未知的 client_id",
		})
		return
	}

	// 生成内部 state，用于关联企业 OAuth 回调
	internalState, _ := generateRandomString(32)

	// 存储 OIDC 请求信息到 cookie
	oidcRequestData := map[string]string{
		"client_id":    clientID,
		"redirect_uri": redirectURI,
		"scope":        scope,
		"state":        state,
		"nonce":        nonce,
	}
	oidcRequestJSON, _ := json.Marshal(oidcRequestData)
	c.SetCookie(
		OIDCStateCookieName,
		base64.URLEncoding.EncodeToString(oidcRequestJSON),
		300, // 5 分钟
		"/",
		p.config.OAuth.CookieDomain,
		p.config.OAuth.CookieSecure,
		true,
	)

	// 重定向到企业 OAuth
	authURL := p.buildUpstreamAuthURL(internalState, scope)
	p.log.Debug("Redirecting to upstream OAuth",
		zap.String("upstreamURL", authURL))

	c.Redirect(http.StatusFound, authURL)
}

// buildUpstreamAuthURL 构建上游 OAuth 授权 URL
func (p *Provider) buildUpstreamAuthURL(state, scope string) string {
	params := url.Values{}
	params.Set("client_id", p.config.OAuth.ClientID)
	params.Set("redirect_uri", p.config.OIDCProvider.IssuerURL+"/oidc/callback")
	params.Set("response_type", "code")
	params.Set("state", state)

	// 使用配置的 scopes
	if len(p.config.OAuth.Scopes) > 0 {
		params.Set("scope", strings.Join(p.config.OAuth.Scopes, " "))
	} else {
		params.Set("scope", scope)
	}

	return p.config.OAuth.AuthorizationEndpoint + "?" + params.Encode()
}

// OAuthCallback 处理企业 OAuth 回调（内部使用）
func (p *Provider) OAuthCallback(c *gin.Context) {
	p.log.Info("OAuth callback received",
		zap.String("clientIP", c.ClientIP()))

	// 获取授权码
	code := c.Query("code")
	if code == "" {
		errParam := c.Query("error")
		errDesc := c.Query("error_description")
		p.log.Error("OAuth callback error",
			zap.String("error", errParam),
			zap.String("description", errDesc))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             errParam,
			"error_description": errDesc,
		})
		return
	}

	// 获取存储的 OIDC 请求信息
	oidcStateCookie, err := c.Cookie(OIDCStateCookieName)
	if err != nil {
		p.log.Warn("Missing OIDC state cookie")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "缺少 OIDC 状态信息",
		})
		return
	}

	// 清除 cookie
	c.SetCookie(OIDCStateCookieName, "", -1, "/", p.config.OAuth.CookieDomain, p.config.OAuth.CookieSecure, true)

	// 解析 OIDC 请求信息
	oidcRequestJSON, _ := base64.URLEncoding.DecodeString(oidcStateCookie)
	var oidcRequest map[string]string
	json.Unmarshal(oidcRequestJSON, &oidcRequest)

	p.log.Debug("Exchanging upstream token")

	// 用企业 OAuth 的 code 换取 token
	token, err := p.exchangeUpstreamToken(c.Request.Context(), code)
	if err != nil {
		p.log.Error("Failed to exchange upstream token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":             "server_error",
			"error_description": fmt.Sprintf("Token 交换失败: %v", err),
		})
		return
	}

	p.log.Debug("Fetching upstream user info")

	// 获取用户信息
	userInfo, err := p.fetchUpstreamUserInfo(c.Request.Context(), token.AccessToken)
	if err != nil {
		p.log.Error("Failed to fetch upstream user info", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":             "server_error",
			"error_description": fmt.Sprintf("获取用户信息失败: %v", err),
		})
		return
	}

	p.log.Info("User authenticated via upstream OAuth",
		zap.String("username", userInfo.PreferredUsername),
		zap.String("email", userInfo.Email),
		zap.String("sub", userInfo.Sub))

	// 生成 OIDC 授权码
	authCode, _ := generateRandomString(32)
	scopes := strings.Split(oidcRequest["scope"], " ")

	p.authCodeMutex.Lock()
	p.authCodes[authCode] = &AuthCode{
		Code:        authCode,
		ClientID:    oidcRequest["client_id"],
		RedirectURI: oidcRequest["redirect_uri"],
		Username:    userInfo.PreferredUsername,
		Email:       userInfo.Email,
		Scopes:      scopes,
		ExpiresAt:   time.Now().Add(AuthCodeDuration),
		Nonce:       oidcRequest["nonce"],
	}
	p.authCodeMutex.Unlock()

	p.log.Debug("Authorization code generated",
		zap.String("clientID", oidcRequest["client_id"]),
		zap.String("username", userInfo.PreferredUsername))

	// 重定向回客户端
	redirectURL, _ := url.Parse(oidcRequest["redirect_uri"])
	q := redirectURL.Query()
	q.Set("code", authCode)
	if oidcRequest["state"] != "" {
		q.Set("state", oidcRequest["state"])
	}
	redirectURL.RawQuery = q.Encode()

	p.log.Debug("Redirecting to client",
		zap.String("redirectURL", redirectURL.String()))

	c.Redirect(http.StatusFound, redirectURL.String())
}

// UpstreamTokenResponse 上游 OAuth Token 响应
type UpstreamTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// exchangeUpstreamToken 用企业 OAuth 的 code 换取 token
func (p *Provider) exchangeUpstreamToken(ctx context.Context, code string) (*UpstreamTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", p.config.OIDCProvider.IssuerURL+"/oidc/callback")
	data.Set("client_id", p.config.OAuth.ClientID)
	data.Set("client_secret", p.config.OAuth.ClientSecret)

	p.log.Debug("Sending upstream token request",
		zap.String("tokenEndpoint", p.config.OAuth.TokenEndpoint))

	req, err := http.NewRequestWithContext(ctx, "POST", p.config.OAuth.TokenEndpoint, strings.NewReader(data.Encode()))
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
		p.log.Error("Upstream token endpoint error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)))
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var token UpstreamTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	p.log.Debug("Upstream token received",
		zap.String("tokenType", token.TokenType),
		zap.Int("expiresIn", token.ExpiresIn))

	return &token, nil
}

// UpstreamUserInfo 上游用户信息
type UpstreamUserInfo struct {
	Sub               string   `json:"sub"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Groups            []string `json:"groups"`
}

// fetchUpstreamUserInfo 从企业 OAuth 获取用户信息
func (p *Provider) fetchUpstreamUserInfo(ctx context.Context, accessToken string) (*UpstreamUserInfo, error) {
	if p.config.OAuth.UserinfoEndpoint == "" {
		return nil, fmt.Errorf("未配置 userinfo endpoint")
	}

	var req *http.Request
	var err error

	// 根据配置使用 GET 或 POST
	method := strings.ToUpper(p.config.OAuth.UserinfoMethod)
	if method == "POST" {
		// POST JSON 方式
		reqBody := map[string]interface{}{
			"client_id":    p.config.OAuth.ClientID,
			"access_token": accessToken,
			"scope":        strings.Join(p.config.OAuth.Scopes, " "),
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req, err = http.NewRequestWithContext(ctx, "POST", p.config.OAuth.UserinfoEndpoint, strings.NewReader(string(bodyBytes)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		// GET 方式（标准 OIDC）
		req, err = http.NewRequestWithContext(ctx, "GET", p.config.OAuth.UserinfoEndpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	p.log.Debug("Fetching upstream user info",
		zap.String("endpoint", p.config.OAuth.UserinfoEndpoint),
		zap.String("method", method))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		p.log.Error("Upstream userinfo endpoint error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)))
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	// 解析为 map 以支持字段映射
	var rawData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawData); err != nil {
		return nil, err
	}

	// 提取用户信息（支持字段映射）
	userInfo := &UpstreamUserInfo{}

	// 用户名
	usernameClaim := p.config.OAuth.TokenUsernameClaim
	if usernameClaim == "" {
		usernameClaim = "preferred_username"
	}
	if v, ok := rawData[usernameClaim].(string); ok {
		userInfo.PreferredUsername = v
	}
	if userInfo.PreferredUsername == "" {
		if v, ok := rawData["username"].(string); ok {
			userInfo.PreferredUsername = v
		}
	}
	if userInfo.PreferredUsername == "" {
		if v, ok := rawData["name"].(string); ok {
			userInfo.PreferredUsername = v
		}
	}

	// 邮箱
	emailClaim := p.config.OAuth.TokenEmailClaim
	if emailClaim == "" {
		emailClaim = "email"
	}
	if v, ok := rawData[emailClaim].(string); ok {
		userInfo.Email = v
	}

	// Name
	if v, ok := rawData["name"].(string); ok {
		userInfo.Name = v
	}

	// Sub
	if v, ok := rawData["sub"].(string); ok {
		userInfo.Sub = v
	} else if v, ok := rawData["id"].(string); ok {
		userInfo.Sub = v
	} else if v, ok := rawData["user_id"].(string); ok {
		userInfo.Sub = v
	} else {
		userInfo.Sub = userInfo.PreferredUsername
	}

	// Groups
	if v, ok := rawData["groups"].([]interface{}); ok {
		for _, g := range v {
			if gs, ok := g.(string); ok {
				userInfo.Groups = append(userInfo.Groups, gs)
			}
		}
	}

	p.log.Debug("Upstream user info parsed",
		zap.String("username", userInfo.PreferredUsername),
		zap.String("email", userInfo.Email),
		zap.Int("groupCount", len(userInfo.Groups)))

	return userInfo, nil
}

// Token Token 端点
func (p *Provider) Token(c *gin.Context) {
	grantType := c.PostForm("grant_type")

	p.log.Debug("Token request received",
		zap.String("grantType", grantType),
		zap.String("clientIP", c.ClientIP()))

	switch grantType {
	case "authorization_code":
		p.handleAuthorizationCodeGrant(c)
	case "refresh_token":
		p.handleRefreshTokenGrant(c)
	default:
		p.log.Warn("Unsupported grant_type", zap.String("grantType", grantType))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "unsupported_grant_type",
			"error_description": "不支持的 grant_type",
		})
	}
}

// handleAuthorizationCodeGrant 处理授权码换取 Token
func (p *Provider) handleAuthorizationCodeGrant(c *gin.Context) {
	code := c.PostForm("code")
	clientID := c.PostForm("client_id")
	clientSecret := c.PostForm("client_secret")
	redirectURI := c.PostForm("redirect_uri")

	// 如果没有在 body 中，尝试从 Basic Auth 获取
	if clientID == "" || clientSecret == "" {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Basic ") {
			decoded, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				clientID = parts[0]
				clientSecret = parts[1]
			}
		}
	}

	p.log.Debug("Authorization code grant request",
		zap.String("clientID", clientID),
		zap.String("redirectURI", redirectURI))

	// 验证授权码
	p.authCodeMutex.Lock()
	authCode, exists := p.authCodes[code]
	if exists {
		delete(p.authCodes, code) // 授权码只能使用一次
	}
	p.authCodeMutex.Unlock()

	if !exists {
		p.log.Warn("Invalid authorization code")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "无效的授权码",
		})
		return
	}

	if time.Now().After(authCode.ExpiresAt) {
		p.log.Warn("Authorization code expired",
			zap.String("username", authCode.Username))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "授权码已过期",
		})
		return
	}

	if authCode.ClientID != clientID {
		p.log.Warn("Client ID mismatch",
			zap.String("expected", authCode.ClientID),
			zap.String("received", clientID))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "client_id 不匹配",
		})
		return
	}

	if authCode.RedirectURI != redirectURI {
		p.log.Warn("Redirect URI mismatch",
			zap.String("expected", authCode.RedirectURI),
			zap.String("received", redirectURI))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "redirect_uri 不匹配",
		})
		return
	}

	// 验证 client_secret
	expectedSecret := p.config.OIDCProvider.KubernetesClientSecret
	if clientID == p.config.OAuth.ClientID {
		expectedSecret = p.config.OAuth.ClientSecret
	}
	if clientSecret != expectedSecret {
		p.log.Warn("Invalid client secret", zap.String("clientID", clientID))
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":             "invalid_client",
			"error_description": "client_secret 错误",
		})
		return
	}

	// 自动创建用户 RBAC（如果启用）
	if p.config.UserRBAC.Enabled && p.config.UserRBAC.AutoCreate && p.k8sClient != nil {
		p.log.Info("Creating user RBAC",
			zap.String("username", authCode.Username),
			zap.String("email", authCode.Email))
		if err := p.ensureUserRBAC(c.Request.Context(), authCode.Username, authCode.Email); err != nil {
			p.log.Warn("Failed to create user RBAC",
				zap.String("username", authCode.Username),
				zap.Error(err))
		} else {
			p.log.Info("User RBAC created successfully",
				zap.String("username", authCode.Username))
		}
	}

	// 生成 Token
	tokenResponse := p.generateTokens(authCode.Username, authCode.Email, authCode.Scopes, authCode.Nonce, clientID)

	p.log.Info("Tokens issued",
		zap.String("username", authCode.Username),
		zap.String("clientID", clientID),
		zap.Duration("accessTokenDuration", AccessTokenDuration),
		zap.Duration("refreshTokenDuration", RefreshTokenDuration))

	c.JSON(http.StatusOK, tokenResponse)
}

// handleRefreshTokenGrant 处理 Refresh Token 换取新 Token
func (p *Provider) handleRefreshTokenGrant(c *gin.Context) {
	refreshToken := c.PostForm("refresh_token")
	clientID := c.PostForm("client_id")
	clientSecret := c.PostForm("client_secret")

	p.log.Debug("Refresh token grant request",
		zap.String("clientID", clientID))

	// 验证 Refresh Token
	p.refreshTokenMutex.RLock()
	tokenData, exists := p.refreshTokens[refreshToken]
	p.refreshTokenMutex.RUnlock()

	if !exists || time.Now().After(tokenData.ExpiresAt) {
		p.log.Warn("Invalid or expired refresh token")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "无效的 refresh_token",
		})
		return
	}

	if tokenData.ClientID != clientID {
		p.log.Warn("Client ID mismatch for refresh token",
			zap.String("expected", tokenData.ClientID),
			zap.String("received", clientID))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "client_id 不匹配",
		})
		return
	}

	// 验证 client_secret
	expectedSecret := p.config.OIDCProvider.KubernetesClientSecret
	if clientID == p.config.OAuth.ClientID {
		expectedSecret = p.config.OAuth.ClientSecret
	}
	if clientSecret != expectedSecret {
		p.log.Warn("Invalid client secret for refresh token", zap.String("clientID", clientID))
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":             "invalid_client",
			"error_description": "client_secret 错误",
		})
		return
	}

	// 生成新 Token
	tokenResponse := p.generateTokens(tokenData.Username, tokenData.Email, tokenData.Scopes, "", clientID)

	p.log.Info("Tokens refreshed",
		zap.String("username", tokenData.Username),
		zap.String("clientID", clientID))

	c.JSON(http.StatusOK, tokenResponse)
}

// generateTokens 生成 Token 响应
func (p *Provider) generateTokens(username, email string, scopes []string, nonce, clientID string) *TokenResponse {
	now := time.Now()
	issuer := p.GetIssuerURL()

	// 生成 ID Token
	idTokenClaims := IDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   username,
			Audience:  jwt.ClaimStrings{clientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(IDTokenDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Nonce:             nonce,
		Email:             email,
		EmailVerified:     true,
		PreferredUsername: username,
		Name:              username,
	}

	idToken := jwt.NewWithClaims(jwt.SigningMethodRS256, idTokenClaims)
	idToken.Header["kid"] = p.keyManager.GetKeyID()
	idTokenString, _ := idToken.SignedString(p.keyManager.GetPrivateKey())

	// 生成 Access Token（也是 JWT）
	accessTokenClaims := jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   username,
		Audience:  jwt.ClaimStrings{clientID},
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenDuration)),
		IssuedAt:  jwt.NewNumericDate(now),
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodRS256, accessTokenClaims)
	accessToken.Header["kid"] = p.keyManager.GetKeyID()
	accessTokenString, _ := accessToken.SignedString(p.keyManager.GetPrivateKey())

	// 生成 Refresh Token
	refreshTokenString, _ := generateRandomString(64)
	p.refreshTokenMutex.Lock()
	p.refreshTokens[refreshTokenString] = &RefreshTokenData{
		Token:     refreshTokenString,
		Username:  username,
		Email:     email,
		ClientID:  clientID,
		Scopes:    scopes,
		ExpiresAt: now.Add(RefreshTokenDuration),
	}
	p.refreshTokenMutex.Unlock()

	return &TokenResponse{
		AccessToken:  accessTokenString,
		TokenType:    "Bearer",
		ExpiresIn:    int(AccessTokenDuration.Seconds()),
		RefreshToken: refreshTokenString,
		IDToken:      idTokenString,
		Scope:        strings.Join(scopes, " "),
	}
}

// UserInfo UserInfo 端点
func (p *Provider) UserInfo(c *gin.Context) {
	// 获取 Access Token
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		p.log.Warn("Missing Bearer token in userinfo request")
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":             "invalid_token",
			"error_description": "缺少 Bearer Token",
		})
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// 验证 Token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return p.keyManager.GetPublicKey(), nil
	})

	if err != nil || !token.Valid {
		p.log.Warn("Invalid token in userinfo request", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":             "invalid_token",
			"error_description": "Token 无效或已过期",
		})
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		p.log.Error("Failed to parse token claims")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":             "server_error",
			"error_description": "无法解析 Token",
		})
		return
	}

	sub, _ := claims["sub"].(string)

	p.log.Debug("UserInfo request",
		zap.String("sub", sub),
		zap.String("clientIP", c.ClientIP()))

	// 返回用户信息
	userInfo := UserInfoResponse{
		Sub:               sub,
		Name:              sub,
		PreferredUsername: sub,
	}

	c.JSON(http.StatusOK, userInfo)
}

// ensureUserRBAC 确保用户的 RBAC 资源存在
func (p *Provider) ensureUserRBAC(ctx context.Context, username, email string) error {
	if p.k8sClient == nil {
		return fmt.Errorf("K8s client not initialized")
	}

	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	return p.k8sClient.EnsureUserRBAC(ctx, k8s.UserRBACConfig{
		Username:  username,
		Email:     email,
		Namespace: namespace,
	})
}

// generateRandomString 生成随机字符串
func generateRandomString(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:length], nil
}

// GetKeyManager 获取密钥管理器（用于调试）
func (p *Provider) GetKeyManager() *KeyManager {
	return p.keyManager
}
