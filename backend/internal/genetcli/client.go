package genetcli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/models"
)

type APIClient struct {
	baseURL    string
	httpClient *http.Client
	configPath string
	config     *Config
}

func NewAPIClient(baseURL string, cfg *Config, configPath string) *APIClient {
	return &APIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		configPath: configPath,
		config:     cfg,
	}
}

func (c *APIClient) DoJSON(ctx context.Context, method, path string, reqBody any, respOut any) error {
	payload, err := marshalBody(reqBody)
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, method, path, payload)
	if err == nil {
		return decodeResponse(resp, respOut)
	}
	if !isUnauthorized(err) {
		return err
	}
	if err := c.refresh(ctx); err != nil {
		return err
	}
	resp, err = c.do(ctx, method, path, payload)
	if err != nil {
		return err
	}
	return decodeResponse(resp, respOut)
}

func (c *APIClient) do(ctx context.Context, method, path string, payload []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.config != nil && c.config.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AccessToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return nil, &apiError{StatusCode: resp.StatusCode, Message: extractErrorMessage(body, resp.Status)}
}

func (c *APIClient) refresh(ctx context.Context) error {
	payload, err := json.Marshal(map[string]string{"refreshToken": c.config.RefreshToken})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/cli/auth/refresh", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &apiError{StatusCode: resp.StatusCode, Message: extractErrorMessage(body, resp.Status)}
	}
	var refreshed models.CLIAuthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshed); err != nil {
		return err
	}
	c.config.AccessToken = refreshed.AccessToken
	c.config.RefreshToken = refreshed.RefreshToken
	c.config.ExpiresAt = refreshed.ExpiresAt
	c.config.Username = refreshed.Username
	c.config.Email = refreshed.Email
	return SaveConfig(c.configPath, c.config)
}

type apiError struct {
	StatusCode int
	Message    string
}

func (e *apiError) Error() string {
	return e.Message
}

func isUnauthorized(err error) bool {
	var apiErr *apiError
	return errorAs(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized
}

func errorAs(err error, target **apiError) bool {
	apiErr, ok := err.(*apiError)
	if !ok {
		return false
	}
	*target = apiErr
	return true
}

func marshalBody(reqBody any) ([]byte, error) {
	if reqBody == nil {
		return nil, nil
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func decodeResponse(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func extractErrorMessage(body []byte, fallback string) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if value, ok := payload["error"].(string); ok && value != "" {
			return value
		}
	}
	if len(body) > 0 {
		return strings.TrimSpace(string(body))
	}
	return fallback
}
