package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/uc-package/genet/internal/models"
)

// oauthHandler 用于验证 session 的 OAuth 处理器
var oauthHandler *OAuthHandler

// InitAuthMiddleware 初始化认证中间件（需要在应用启动时调用）
func InitAuthMiddleware(config *models.Config) {
	oauthHandler = NewOAuthHandler(config)
}

// AuthMiddleware 认证中间件
func AuthMiddleware(config *models.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var username, email string

		// 1. 首先尝试从 session cookie 获取用户信息（OAuth 登录后）
		if sessionToken, err := c.Cookie(SessionCookieName); err == nil && sessionToken != "" {
			claims, err := validateSessionToken(sessionToken, config.OAuth.JWTSecret)
			if err == nil {
				username = claims.Username
				email = claims.Email
			}
		}

		// 2. 如果没有 session，尝试从 OAuth2 Proxy 注入的 HTTP 头获取（兼容模式）
		if username == "" {
			username = c.GetHeader("X-Auth-Request-User")
			if username == "" {
				username = c.GetHeader("X-Auth-Request-Preferred-Username")
			}
			email = c.GetHeader("X-Auth-Request-Email")
		}

		// 3. 如果还是没有用户信息，使用默认开发用户（仅在 OAuth 未启用时）
		if username == "" {
			if !config.OAuth.Enabled {
				// 开发模式：从查询参数获取用户名
				username = c.Query("user")
				if username == "" {
					username = "dev-user" // 默认开发用户
				}
				email = username + "@example.com"
			}
		}

		// 将用户信息存储到上下文
		c.Set("username", username)
		c.Set("email", email)
		c.Set("authenticated", username != "" && username != "dev-user")

		c.Next()
	}
}

// validateSessionToken 验证 session token
func validateSessionToken(tokenString, secret string) (*SessionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &SessionClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*SessionClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrSignatureInvalid
}

// GetUsername 从上下文获取用户名
func GetUsername(c *gin.Context) (string, bool) {
	username, exists := c.Get("username")
	if !exists {
		return "", false
	}
	return username.(string), true
}

// GetEmail 从上下文获取邮箱
func GetEmail(c *gin.Context) (string, bool) {
	email, exists := c.Get("email")
	if !exists {
		return "", false
	}
	return email.(string), true
}

// RequireAuth 要求认证
func RequireAuth(c *gin.Context) {
	username, exists := GetUsername(c)
	if !exists || username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权：缺少用户信息"})
		c.Abort()
		return
	}
}

