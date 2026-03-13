package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestCLIAuthStartCreatesRequestAndReturnsLoginURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	client := k8s.NewClientForTest(k8sfake.NewSimpleClientset(), cfg)
	handler := NewCLIAuthHandler(client, cfg)

	router := gin.New()
	router.POST("/api/cli/auth/start", handler.Start)

	body := `{"codeChallenge":"challenge-1","localCallbackURL":"http://127.0.0.1:51234/callback","state":"state-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/cli/auth/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp models.CLIAuthStartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RequestID == "" {
		t.Fatalf("expected request id")
	}
	if !strings.Contains(resp.LoginURL, "/api/cli/auth/complete?request_id="+resp.RequestID) {
		t.Fatalf("expected loginURL to point to complete endpoint, got %q", resp.LoginURL)
	}

	stored, err := client.GetCLIAuthRequest(context.Background(), resp.RequestID)
	if err != nil {
		t.Fatalf("expected stored auth request: %v", err)
	}
	if stored.State != "state-1" {
		t.Fatalf("expected stored state, got %q", stored.State)
	}
}

func TestCLIAuthCompleteRedirectsUnauthenticatedUsersToOAuthLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	auth.InitAuthMiddleware(cfg)
	client := k8s.NewClientForTest(k8sfake.NewSimpleClientset(), cfg)
	handler := NewCLIAuthHandler(client, cfg)

	record := models.CLIAuthRequestRecord{
		ID:               "req-1",
		CodeChallenge:    pkceChallenge("verifier-1"),
		LocalCallbackURL: "http://127.0.0.1:51234/callback",
		State:            "state-1",
		ExpiresAt:        time.Now().Add(5 * time.Minute),
	}
	if err := client.CreateCLIAuthRequest(context.Background(), record); err != nil {
		t.Fatalf("seed auth request: %v", err)
	}

	router := gin.New()
	router.GET("/api/cli/auth/complete", auth.AuthMiddleware(cfg), handler.Complete)

	req := httptest.NewRequest(http.MethodGet, "/api/cli/auth/complete?request_id=req-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/api/auth/login?return_to=") {
		t.Fatalf("expected oauth login redirect, got %q", location)
	}
}

func TestCLIAuthCompleteRedirectsAuthenticatedUsersToLocalCallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	auth.InitAuthMiddleware(cfg)
	client := k8s.NewClientForTest(k8sfake.NewSimpleClientset(), cfg)
	handler := NewCLIAuthHandler(client, cfg)

	record := models.CLIAuthRequestRecord{
		ID:               "req-2",
		CodeChallenge:    pkceChallenge("verifier-2"),
		LocalCallbackURL: "http://127.0.0.1:51234/callback",
		State:            "state-2",
		ExpiresAt:        time.Now().Add(5 * time.Minute),
	}
	if err := client.CreateCLIAuthRequest(context.Background(), record); err != nil {
		t.Fatalf("seed auth request: %v", err)
	}

	oauthHandler := auth.NewOAuthHandler(cfg)
	sessionToken, err := oauthHandler.CreateSessionTokenForTest("alice", "alice@example.com")
	if err != nil {
		t.Fatalf("create test session: %v", err)
	}

	router := gin.New()
	router.GET("/api/cli/auth/complete", auth.AuthMiddleware(cfg), handler.Complete)

	req := httptest.NewRequest(http.MethodGet, "/api/cli/auth/complete?request_id=req-2", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionToken})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "http://127.0.0.1:51234/callback?") {
		t.Fatalf("expected callback redirect, got %q", location)
	}
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if parsed.Query().Get("state") != "state-2" {
		t.Fatalf("expected callback state state-2, got %q", parsed.Query().Get("state"))
	}
	if parsed.Query().Get("code") == "" {
		t.Fatalf("expected auth code in callback redirect")
	}
}

func TestCLIAuthExchangeRefreshAndLogout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	client := k8s.NewClientForTest(k8sfake.NewSimpleClientset(), cfg)
	handler := NewCLIAuthHandler(client, cfg)

	record := models.CLIAuthRequestRecord{
		ID:               "req-3",
		CodeChallenge:    pkceChallenge("verifier-3"),
		LocalCallbackURL: "http://127.0.0.1:51234/callback",
		State:            "state-3",
		Username:         "alice",
		Email:            "alice@example.com",
		AuthCodeHash:     k8s.HashCLISecretForTest("one-time-code"),
		ExpiresAt:        time.Now().Add(5 * time.Minute),
	}
	if err := client.CreateCLIAuthRequest(context.Background(), record); err != nil {
		t.Fatalf("seed auth request: %v", err)
	}

	router := gin.New()
	router.POST("/api/cli/auth/exchange", handler.Exchange)
	router.POST("/api/cli/auth/refresh", handler.Refresh)
	router.POST("/api/cli/auth/logout", handler.Logout)

	exchangeReq := `{"requestID":"req-3","code":"one-time-code","codeVerifier":"verifier-3"}`
	exchangeRec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cli/auth/exchange", strings.NewReader(exchangeReq))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(exchangeRec, req)

	if exchangeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from exchange, got %d body=%s", exchangeRec.Code, exchangeRec.Body.String())
	}

	var tokenResp models.CLIAuthTokenResponse
	if err := json.Unmarshal(exchangeRec.Body.Bytes(), &tokenResp); err != nil {
		t.Fatalf("decode exchange response: %v", err)
	}
	if tokenResp.AccessToken == "" || tokenResp.RefreshToken == "" {
		t.Fatalf("expected access and refresh tokens")
	}

	found, ok, err := client.FindCLIRefreshSessionByPlaintext(context.Background(), tokenResp.RefreshToken)
	if err != nil || !ok {
		t.Fatalf("expected persisted refresh session, ok=%v err=%v", ok, err)
	}

	refreshRec := httptest.NewRecorder()
	refreshReq := httptest.NewRequest(http.MethodPost, "/api/cli/auth/refresh", strings.NewReader(`{"refreshToken":"`+tokenResp.RefreshToken+`"}`))
	refreshReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(refreshRec, refreshReq)

	if refreshRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from refresh, got %d body=%s", refreshRec.Code, refreshRec.Body.String())
	}
	var refreshed models.CLIAuthTokenResponse
	if err := json.Unmarshal(refreshRec.Body.Bytes(), &refreshed); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if refreshed.RefreshToken == "" || refreshed.RefreshToken == tokenResp.RefreshToken {
		t.Fatalf("expected rotated refresh token")
	}

	logoutRec := httptest.NewRecorder()
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/cli/auth/logout", strings.NewReader(`{"refreshToken":"`+refreshed.RefreshToken+`"}`))
	logoutReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from logout, got %d body=%s", logoutRec.Code, logoutRec.Body.String())
	}

	if _, ok, err := client.FindCLIRefreshSessionByPlaintext(context.Background(), refreshed.RefreshToken); err != nil || ok {
		t.Fatalf("expected revoked refresh token to stop matching, ok=%v err=%v", ok, err)
	}
	if found.ID == "" {
		t.Fatalf("expected original refresh session id")
	}
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
