package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/models"
)

func TestIsAdmin_MatchesUsernameOrEmail(t *testing.T) {
	cfg := &models.Config{
		AdminUsers: []string{"alice", "bob@example.com"},
	}

	if !IsAdmin(cfg, "alice", "alice@example.com") {
		t.Fatalf("expected username to match admin list")
	}
	if !IsAdmin(cfg, "bob", "bob@example.com") {
		t.Fatalf("expected email to match admin list")
	}
	if IsAdmin(cfg, "charlie", "charlie@example.com") {
		t.Fatalf("expected non-admin user")
	}
}

func TestRequireAdmin_RejectsNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &models.Config{
		AdminUsers: []string{"alice"},
	}

	r := gin.New()
	r.GET("/admin", func(c *gin.Context) {
		c.Set("username", "bob")
		c.Set("email", "bob@example.com")
		RequireAdmin(cfg)(c)
		if c.IsAborted() {
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestRequireAdmin_AllowsAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &models.Config{
		AdminUsers: []string{"alice"},
	}

	r := gin.New()
	r.GET("/admin", func(c *gin.Context) {
		c.Set("username", "alice")
		c.Set("email", "alice@example.com")
		RequireAdmin(cfg)(c)
		if c.IsAborted() {
			return
		}
		isAdmin, _ := c.Get("isAdmin")
		c.JSON(http.StatusOK, gin.H{"isAdmin": isAdmin})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
