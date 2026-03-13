package genetcli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginRunsPKCEFlowAndPersistsConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	configPath, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("default config path: %v", err)
	}

	var exchanged struct {
		RequestID    string
		Code         string
		CodeVerifier string
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/cli/auth/start":
			var req struct {
				CodeChallenge    string `json:"codeChallenge"`
				LocalCallbackURL string `json:"localCallbackURL"`
				State            string `json:"state"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode start request: %v", err)
			}
			if req.CodeChallenge == "" || req.LocalCallbackURL == "" || req.State == "" {
				t.Fatalf("expected populated start request: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"requestID": "req-1",
				"loginURL":  server.URL + "/browser-login?callback=" + req.LocalCallbackURL + "&state=" + req.State,
				"expiresAt": time.Now().Add(5 * time.Minute).UTC(),
			})
		case "/browser-login":
			callbackURL := r.URL.Query().Get("callback")
			state := r.URL.Query().Get("state")
			go func() {
				_, _ = http.Get(callbackURL + "?code=auth-code-1&state=" + state)
			}()
			w.WriteHeader(http.StatusNoContent)
		case "/api/cli/auth/exchange":
			if err := json.NewDecoder(r.Body).Decode(&exchanged); err != nil {
				t.Fatalf("decode exchange request: %v", err)
			}
			if exchanged.RequestID != "req-1" || exchanged.Code != "auth-code-1" || exchanged.CodeVerifier == "" {
				t.Fatalf("unexpected exchange payload: %+v", exchanged)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "access-1",
				"refreshToken": "refresh-1",
				"expiresAt":    time.Now().Add(15 * time.Minute).UTC(),
				"username":     "alice",
				"email":        "alice@example.com",
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := Login(ctx, LoginOptions{
		Server:     server.URL,
		ConfigPath: configPath,
		OpenBrowser: func(target string) error {
			resp, err := http.Get(target)
			if err != nil {
				return err
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			return resp.Body.Close()
		},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if cfg.AccessToken != "access-1" || cfg.RefreshToken != "refresh-1" || cfg.Username != "alice" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if exchanged.CodeVerifier == "" {
		t.Fatalf("expected PKCE code verifier in exchange request")
	}

	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load persisted config: %v", err)
	}
	if loaded.AccessToken != "access-1" || loaded.Server != server.URL {
		t.Fatalf("unexpected persisted config: %+v", loaded)
	}
}
