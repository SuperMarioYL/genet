package genetcli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/models"
)

type LoginOptions struct {
	Server      string
	ConfigPath  string
	OpenBrowser func(string) error
	HTTPClient  *http.Client
}

type loginCallback struct {
	code  string
	state string
}

func Login(ctx context.Context, opts LoginOptions) (*Config, error) {
	configPath := opts.ConfigPath
	if configPath == "" {
		var err error
		configPath, err = DefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	openBrowser := opts.OpenBrowser
	if openBrowser == nil {
		openBrowser = defaultBrowserOpener
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()

	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		return nil, err
	}
	state, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	callbackCh := make(chan loginCallback, 1)
	serverErrCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		callbackCh <- loginCallback{
			code:  r.URL.Query().Get("code"),
			state: r.URL.Query().Get("state"),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, "<html><body>Genet CLI login complete. You can close this tab.</body></html>")
	})
	callbackServer := &http.Server{Handler: mux}
	go func() {
		serverErrCh <- callbackServer.Serve(listener)
	}()

	startReq := models.CLIAuthStartRequest{
		CodeChallenge:    pkceChallenge(codeVerifier),
		LocalCallbackURL: "http://" + listener.Addr().String(),
		State:            state,
	}
	var startResp models.CLIAuthStartResponse
	if err := doJSON(ctx, httpClient, http.MethodPost, strings.TrimRight(opts.Server, "/")+"/api/cli/auth/start", startReq, &startResp); err != nil {
		_ = callbackServer.Shutdown(context.Background())
		return nil, err
	}

	if err := openBrowser(startResp.LoginURL); err != nil {
		_ = callbackServer.Shutdown(context.Background())
		return nil, err
	}

	var callback loginCallback
	select {
	case callback = <-callbackCh:
	case err := <-serverErrCh:
		if err != nil && err != http.ErrServerClosed {
			return nil, err
		}
	case <-ctx.Done():
		_ = callbackServer.Shutdown(context.Background())
		return nil, ctx.Err()
	}
	_ = callbackServer.Shutdown(context.Background())

	if callback.state != state {
		return nil, fmt.Errorf("unexpected callback state")
	}

	exchangeReq := models.CLIAuthExchangeRequest{
		RequestID:    startResp.RequestID,
		Code:         callback.code,
		CodeVerifier: codeVerifier,
	}
	var tokenResp models.CLIAuthTokenResponse
	if err := doJSON(ctx, httpClient, http.MethodPost, strings.TrimRight(opts.Server, "/")+"/api/cli/auth/exchange", exchangeReq, &tokenResp); err != nil {
		return nil, err
	}

	cfg := &Config{
		Server:       strings.TrimRight(opts.Server, "/"),
		Username:     tokenResp.Username,
		Email:        tokenResp.Email,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.ExpiresAt,
	}
	if err := SaveConfig(configPath, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaultBrowserOpener(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

func generateCodeVerifier() (string, error) {
	return randomHex(32)
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func doJSON(ctx context.Context, client *http.Client, method, url string, reqBody any, out any) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		if msg, ok := payload["error"].(string); ok && msg != "" {
			return fmt.Errorf("%s", html.UnescapeString(msg))
		}
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
