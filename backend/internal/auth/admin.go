package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
)

// IsAdmin 判断当前用户是否在管理员白名单中
func IsAdmin(config *models.Config, username, email string) bool {
	if config == nil || len(config.AdminUsers) == 0 {
		return false
	}

	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)

	for _, admin := range config.AdminUsers {
		admin = strings.TrimSpace(admin)
		if admin == "" {
			continue
		}
		if admin == username || admin == email {
			return true
		}
	}
	return false
}

// RequireAdmin 要求管理员权限
func RequireAdmin(config *models.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		RequireAuth(c)
		if c.IsAborted() {
			return
		}

		username, _ := GetUsername(c)
		email, _ := GetEmail(c)
		if !IsAdmin(config, username, email) {
			if authLog != nil {
				authLog.Warn("Forbidden: admin required",
					zap.String("username", username),
					zap.String("email", email),
					zap.String("path", c.Request.URL.Path))
			}
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: admin access required"})
			c.Abort()
			return
		}

		c.Set("isAdmin", true)
		c.Next()
	}
}
