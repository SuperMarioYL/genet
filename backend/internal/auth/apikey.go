package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
)

// OpenAPIKeyLookupFunc 查找明文 API Key 对应的记录。
type OpenAPIKeyLookupFunc func(ctx context.Context, plaintext string) (*models.APIKeyRecord, bool, error)

// APIKeyAuthMiddleware API Key 认证中间件
// 从 Authorization: Bearer <key> 提取 key，优先走管理页 key，失败则回退静态 key。
func APIKeyAuthMiddleware(config *models.Config, k8sClient *k8s.Client) gin.HandlerFunc {
	var lookup OpenAPIKeyLookupFunc
	if k8sClient != nil {
		lookup = k8sClient.FindOpenAPIKeyByPlaintext
	}
	return APIKeyAuthMiddlewareWithLookup(config, lookup)
}

// APIKeyAuthMiddlewareWithLookup 可注入查找函数，便于单测。
func APIKeyAuthMiddlewareWithLookup(config *models.Config, lookup OpenAPIKeyLookupFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !config.OpenAPI.Enabled {
			c.JSON(http.StatusForbidden, gin.H{"error": "Open API is not enabled"})
			c.Abort()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid Authorization header"})
			c.Abort()
			return
		}

		apiKey := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			c.Abort()
			return
		}

		// 1) 优先使用管理页维护的 key 记录
		if lookup != nil {
			record, found, err := lookup(c.Request.Context(), apiKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate API key"})
				c.Abort()
				return
			}
			if found && k8s.IsOpenAPIKeyActive(record) {
				setOpenAPIAuthContext(c, record)
				c.Next()
				return
			}
		}

		// 2) 回退旧版静态 key
		if matchLegacyAPIKey(config.OpenAPI.APIKeys, apiKey) {
			c.Set("authMethod", "apikey")
			c.Set("openapiScope", models.APIKeyScopeWrite)
			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
		c.Abort()
	}
}

// RequireOpenAPIScope 要求 OpenAPI 请求具备指定 scope。
func RequireOpenAPIScope(required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope := c.GetString("openapiScope")
		if scope == "" {
			// 兼容未设置 scope 的旧请求，按 write 处理
			scope = models.APIKeyScopeWrite
		}
		if !openAPIScopeAllowed(scope, required) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: insufficient scope"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func setOpenAPIAuthContext(c *gin.Context, record *models.APIKeyRecord) {
	c.Set("authMethod", "apikey")
	if record == nil {
		c.Set("openapiScope", models.APIKeyScopeWrite)
		return
	}
	c.Set("openapiKeyID", record.ID)
	c.Set("openapiOwnerUser", record.OwnerUser)
	scope := strings.TrimSpace(record.Scope)
	if scope == "" {
		scope = models.APIKeyScopeRead
	}
	c.Set("openapiScope", scope)
}

func matchLegacyAPIKey(legacyKeys []string, candidate string) bool {
	for _, key := range legacyKeys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(candidate)) == 1 {
			return true
		}
	}
	return false
}

func openAPIScopeAllowed(granted, required string) bool {
	granted = strings.ToLower(strings.TrimSpace(granted))
	required = strings.ToLower(strings.TrimSpace(required))

	if required == "" || required == models.APIKeyScopeRead {
		return granted == models.APIKeyScopeRead || granted == models.APIKeyScopeWrite
	}
	if required == models.APIKeyScopeWrite {
		return granted == models.APIKeyScopeWrite
	}
	return false
}
