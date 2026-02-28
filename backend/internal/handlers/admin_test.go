package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/models"
)

func TestAdminMe_ForbiddenForNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.AdminUsers = []string{"alice"}
	auth.InitAuthMiddleware(cfg)

	h := NewAdminHandler(cfg, nil)
	r := gin.New()
	r.GET("/api/admin/me", auth.AuthMiddleware(cfg), auth.RequireAdmin(cfg), h.GetMe)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	req.Header.Set("X-Auth-Request-User", "bob")
	req.Header.Set("X-Auth-Request-Email", "bob@example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestAdminMe_OKForAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.AdminUsers = []string{"alice"}
	auth.InitAuthMiddleware(cfg)

	h := NewAdminHandler(cfg, nil)
	r := gin.New()
	r.GET("/api/admin/me", auth.AuthMiddleware(cfg), auth.RequireAdmin(cfg), h.GetMe)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	req.Header.Set("X-Auth-Request-User", "alice")
	req.Header.Set("X-Auth-Request-Email", "alice@example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
