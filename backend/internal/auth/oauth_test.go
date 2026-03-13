package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/models"
)

func TestOAuthLoginSetsReturnToCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.OAuth.Mode = ModeOAuth
	cfg.OAuth.AuthorizationEndpoint = "https://oauth.example.com/authorize"
	cfg.OAuth.TokenEndpoint = "https://oauth.example.com/token"
	cfg.OAuth.ClientID = "genet-cli"
	cfg.OAuth.RedirectURL = "http://localhost:8080/api/auth/callback"

	handler := NewOAuthHandler(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/login?return_to=http://127.0.0.1:54321/callback", nil)
	c.Request = req

	handler.Login(c)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	cookies := w.Result().Cookies()
	if !hasCookie(cookies, ReturnToCookieName, "") {
		t.Fatalf("expected %s cookie to be set", ReturnToCookieName)
	}
}

func TestOAuthCallbackRedirectsToReturnTo(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oauthServer := newMockOAuthServer(t)
	defer oauthServer.Close()

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.OAuth.Mode = ModeOAuth
	cfg.OAuth.AuthorizationEndpoint = oauthServer.URL + "/authorize"
	cfg.OAuth.TokenEndpoint = oauthServer.URL + "/token"
	cfg.OAuth.UserinfoEndpoint = oauthServer.URL + "/userinfo"
	cfg.OAuth.ClientID = "genet-cli"
	cfg.OAuth.ClientSecret = "secret"
	cfg.OAuth.RedirectURL = "http://localhost:8080/api/auth/callback"
	cfg.OAuth.FrontendURL = "http://localhost:3000"
	cfg.OAuth.UserinfoSource = UserinfoSourceEndpoint

	handler := NewOAuthHandler(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=test-code", nil)
	req.AddCookie(&http.Cookie{Name: ReturnToCookieName, Value: "http://127.0.0.1:54321/callback"})
	c.Request = req

	handler.Callback(c)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "http://127.0.0.1:54321/callback" {
		t.Fatalf("expected redirect to return_to, got %q", got)
	}
	if !hasCookie(w.Result().Cookies(), SessionCookieName, "") {
		t.Fatalf("expected session cookie to be set")
	}
}

func TestOAuthCallbackFallsBackToFrontendURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oauthServer := newMockOAuthServer(t)
	defer oauthServer.Close()

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.OAuth.Mode = ModeOAuth
	cfg.OAuth.AuthorizationEndpoint = oauthServer.URL + "/authorize"
	cfg.OAuth.TokenEndpoint = oauthServer.URL + "/token"
	cfg.OAuth.UserinfoEndpoint = oauthServer.URL + "/userinfo"
	cfg.OAuth.ClientID = "genet-web"
	cfg.OAuth.ClientSecret = "secret"
	cfg.OAuth.RedirectURL = "http://localhost:8080/api/auth/callback"
	cfg.OAuth.FrontendURL = "http://localhost:3000/app"
	cfg.OAuth.UserinfoSource = UserinfoSourceEndpoint

	handler := NewOAuthHandler(cfg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=test-code", nil)
	c.Request = req

	handler.Callback(c)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if got := w.Header().Get("Location"); got != "http://localhost:3000/app" {
		t.Fatalf("expected redirect to frontend URL, got %q", got)
	}
}

func newMockOAuthServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			if got := r.Form.Get("code"); got != "test-code" {
				t.Fatalf("expected code test-code, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "access-token",
				TokenType:   "Bearer",
				ExpiresIn:   3600,
			})
		case "/userinfo":
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Fatalf("expected bearer token, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"preferred_username": "alice",
				"email":              "alice@example.com",
				"sub":                "user-1",
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
}

func hasCookie(cookies []*http.Cookie, name, expectedValue string) bool {
	for _, cookie := range cookies {
		if cookie.Name != name {
			continue
		}
		if expectedValue == "" || strings.Contains(cookie.Value, expectedValue) || cookie.Value == expectedValue {
			return true
		}
	}
	return false
}
