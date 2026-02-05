package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/models"
)

// APIKeyAuthMiddleware API Key 认证中间件
// 从 Authorization: Bearer <key> 提取 key，与 config.OpenAPI.APIKeys 比对
func APIKeyAuthMiddleware(config *models.Config) gin.HandlerFunc {
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

		apiKey := strings.TrimPrefix(authHeader, "Bearer ")
		valid := false
		for _, key := range config.OpenAPI.APIKeys {
			if key == apiKey {
				valid = true
				break
			}
		}

		if !valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			c.Abort()
			return
		}

		c.Set("authMethod", "apikey")
		c.Next()
	}
}
