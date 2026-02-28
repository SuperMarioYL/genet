package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDeriveCustomNameForUpdate(t *testing.T) {
	userIdentifier := "alice"

	got := deriveCustomNameForUpdate("pod-alice-train", userIdentifier)
	if got != "train" {
		t.Fatalf("expected custom name train, got %q", got)
	}

	got = deriveCustomNameForUpdate("pod-other-train", userIdentifier)
	if got != "" {
		t.Fatalf("expected empty custom name for unmatched prefix, got %q", got)
	}
}

func TestApplyOpenAPIOwnerUserContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// no owner user
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		if _, ok := applyOpenAPIOwnerUserContext(c); ok {
			t.Fatalf("expected owner resolution to fail without openapiOwnerUser")
		}
	}

	// with owner user
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("openapiOwnerUser", "alice")
		userIdentifier, ok := applyOpenAPIOwnerUserContext(c)
		if !ok {
			t.Fatalf("expected owner resolution success")
		}
		if userIdentifier == "" {
			t.Fatalf("expected non-empty userIdentifier")
		}
		username := c.GetString("username")
		if username != "alice" {
			t.Fatalf("expected username alice in context, got %q", username)
		}
	}
}
