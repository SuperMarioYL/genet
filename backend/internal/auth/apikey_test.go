package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/models"
)

func TestAPIKeyAuthMiddlewareWithLookup_SetsOwnerUserAndScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OpenAPI.Enabled = true

	lookup := func(ctx context.Context, plaintext string) (*models.APIKeyRecord, bool, error) {
		if plaintext != "gk_managed" {
			return nil, false, nil
		}
		return &models.APIKeyRecord{
			ID:        "k1",
			OwnerUser: "alice",
			Scope:     models.APIKeyScopeWrite,
			Enabled:   true,
			ExpiresAt: ptrTime(time.Now().Add(1 * time.Hour)),
		}, true, nil
	}

	r := gin.New()
	r.GET("/open", APIKeyAuthMiddlewareWithLookup(cfg, lookup), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"owner": c.GetString("openapiOwnerUser"),
			"scope": c.GetString("openapiScope"),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/open", nil)
	req.Header.Set("Authorization", "Bearer gk_managed")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRequireOpenAPIScope_RejectsWriteWhenReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("openapiScope", models.APIKeyScopeRead)
		c.Next()
	})
	r.POST("/open", RequireOpenAPIScope(models.APIKeyScopeWrite), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/open", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
