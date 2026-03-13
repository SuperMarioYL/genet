package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/models"
)

func TestAuthMiddlewareAcceptsCLIAccessBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	InitAuthMiddleware(cfg)
	token, err := createCLIAccessToken(cfg, "alice", "alice@example.com", "sess-1", defaultCLIAccessTokenTTL)
	if err != nil {
		t.Fatalf("create cli access token: %v", err)
	}

	router := gin.New()
	router.GET("/pods", AuthMiddleware(cfg), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"username":     c.GetString("username"),
			"authenticated": c.GetBool("authenticated"),
			"authMethod":   c.GetString("authMethod"),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/pods", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !containsAll(body, "alice", "true", "cli-token") {
		t.Fatalf("expected cli token auth fields in response, got %s", body)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
