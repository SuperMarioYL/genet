package genetcli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientRefreshesTokenOnceAfter401(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	configPath, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("default config path: %v", err)
	}

	cfg := &Config{
		Server:       "http://placeholder",
		Username:     "alice",
		Email:        "alice@example.com",
		AccessToken:  "expired-token",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/pods":
			attempts++
			authHeader := r.Header.Get("Authorization")
			if attempts == 1 {
				if authHeader != "Bearer expired-token" {
					t.Fatalf("expected first request to use expired token, got %q", authHeader)
				}
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if authHeader != "Bearer access-2" {
				t.Fatalf("expected retried request to use new token, got %q", authHeader)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"pods": []any{}})
		case "/api/cli/auth/refresh":
			var req struct {
				RefreshToken string `json:"refreshToken"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode refresh request: %v", err)
			}
			if req.RefreshToken != "refresh-1" {
				t.Fatalf("expected refresh token refresh-1, got %q", req.RefreshToken)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "access-2",
				"refreshToken": "refresh-2",
				"expiresAt":    time.Now().Add(15 * time.Minute).UTC(),
				"username":     "alice",
				"email":        "alice@example.com",
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	cfg.Server = server.URL
	if err := SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	client := NewAPIClient(server.URL, cfg, configPath)
	var resp struct {
		Pods []any `json:"pods"`
	}
	if err := client.DoJSON(context.Background(), http.MethodGet, "/api/pods", nil, &resp); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", attempts)
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config after refresh: %v", err)
	}
	if loaded.AccessToken != "access-2" || loaded.RefreshToken != "refresh-2" {
		t.Fatalf("expected rotated tokens to persist, got %+v", loaded)
	}
}
