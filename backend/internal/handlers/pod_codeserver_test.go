package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildPodConnectionsIncludesCodeServerWhenReady(t *testing.T) {
	handler := newCodeServerTestHandler()
	handler.config.Pod.CodeServer.Enabled = true
	handler.codeServerProbe = func(_ context.Context, host string, port int32) bool {
		return host == "10.0.0.8" && port == 13337
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-alice-dev",
			Namespace: "user-alice",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.8",
		},
	}

	connections := handler.buildPodConnections(context.Background(), pod)

	if connections == nil {
		t.Fatalf("expected connections")
	}
	if connections.Apps.CodeServerURL != "/api/pods/pod-alice-dev/apps/code-server" {
		t.Fatalf("unexpected code-server url: %q", connections.Apps.CodeServerURL)
	}
	if !connections.Apps.CodeServerReady {
		t.Fatalf("expected code-server ready")
	}
	if connections.Apps.CodeServerStatus != "enabled" {
		t.Fatalf("expected enabled status, got %q", connections.Apps.CodeServerStatus)
	}
}

func TestBuildPodConnectionsOmitsCodeServerWhenDisabled(t *testing.T) {
	handler := newCodeServerTestHandler()
	handler.config.Pod.CodeServer.Enabled = false

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-alice-dev"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.8",
		},
	}

	connections := handler.buildPodConnections(context.Background(), pod)

	if connections == nil {
		t.Fatalf("expected connections")
	}
	if connections.Apps.CodeServerURL != "" {
		t.Fatalf("expected empty code-server url, got %q", connections.Apps.CodeServerURL)
	}
	if connections.Apps.CodeServerReady {
		t.Fatalf("expected code-server not ready when disabled")
	}
	if connections.Apps.CodeServerStatus != "unavailable" {
		t.Fatalf("expected unavailable status, got %q", connections.Apps.CodeServerStatus)
	}
}

func TestBuildPodConnectionsMarksCodeServerStartingWhenProbeFails(t *testing.T) {
	handler := newCodeServerTestHandler()
	handler.config.Pod.CodeServer.Enabled = true
	handler.codeServerProbe = func(context.Context, string, int32) bool {
		return false
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-alice-dev"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.8",
		},
	}

	connections := handler.buildPodConnections(context.Background(), pod)

	if connections.Apps.CodeServerURL == "" {
		t.Fatalf("expected code-server url to still be exposed while starting")
	}
	if connections.Apps.CodeServerReady {
		t.Fatalf("expected code-server not ready")
	}
	if connections.Apps.CodeServerStatus != "starting" {
		t.Fatalf("expected starting status, got %q", connections.Apps.CodeServerStatus)
	}
}

func TestProxyCodeServerRejectsNonRunningPod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.Pod.CodeServer.Enabled = true
	auth.InitAuthMiddleware(cfg)

	handler := newCodeServerTestHandler()
	handler.config = cfg
	handler.getPodFn = func(context.Context, string, string) (*corev1.Pod, error) {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-alice-dev", Namespace: "user-alice"},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}, nil
	}

	router := gin.New()
	router.Any("/api/pods/:id/apps/code-server", auth.AuthMiddleware(cfg), handler.ProxyCodeServer)

	req := httptest.NewRequest(http.MethodGet, "/api/pods/pod-alice-dev/apps/code-server", nil)
	req.Header.Set("X-Auth-Request-User", "alice")
	req.Header.Set("X-Auth-Request-Email", "alice@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rec.Code)
	}
}

func TestProxyCodeServerRejectsMissingPodIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.Pod.CodeServer.Enabled = true
	auth.InitAuthMiddleware(cfg)

	handler := newCodeServerTestHandler()
	handler.config = cfg
	handler.getPodFn = func(context.Context, string, string) (*corev1.Pod, error) {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-alice-dev", Namespace: "user-alice"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}, nil
	}

	router := gin.New()
	router.Any("/api/pods/:id/apps/code-server", auth.AuthMiddleware(cfg), handler.ProxyCodeServer)

	req := httptest.NewRequest(http.MethodGet, "/api/pods/pod-alice-dev/apps/code-server", nil)
	req.Header.Set("X-Auth-Request-User", "alice")
	req.Header.Set("X-Auth-Request-Email", "alice@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}

func TestProxyCodeServerForwardsPathAndQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.Pod.CodeServer.Enabled = true
	auth.InitAuthMiddleware(cfg)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]string{
			"path":  r.URL.Path,
			"query": r.URL.RawQuery,
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer targetServer.Close()

	targetURL, err := url.Parse(targetServer.URL)
	if err != nil {
		t.Fatalf("failed to parse target url: %v", err)
	}
	host, portText, err := net.SplitHostPort(targetURL.Host)
	if err != nil {
		t.Fatalf("failed to split target host: %v", err)
	}

	handler := newCodeServerTestHandler()
	handler.config = cfg
	handler.config.Pod.CodeServer.Port = 0
	handler.getPodFn = func(context.Context, string, string) (*corev1.Pod, error) {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-alice-dev", Namespace: "user-alice"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: host,
			},
		}, nil
	}
	handler.codeServerTargetURL = func(*corev1.Pod) (*url.URL, error) {
		return url.Parse("http://" + net.JoinHostPort(host, portText))
	}

	router := gin.New()
	router.Any("/api/pods/:id/apps/code-server", auth.AuthMiddleware(cfg), handler.ProxyCodeServer)
	router.Any("/api/pods/:id/apps/code-server/*path", auth.AuthMiddleware(cfg), handler.ProxyCodeServer)
	appServer := httptest.NewServer(router)
	defer appServer.Close()

	req, err := http.NewRequest(http.MethodGet, appServer.URL+"/api/pods/pod-alice-dev/apps/code-server/api/v1/workspace?folder=%2Fworkspace-genet", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("X-Auth-Request-User", "alice")
	req.Header.Set("X-Auth-Request-Email", "alice@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload["path"] != "/api/v1/workspace" {
		t.Fatalf("expected forwarded path /api/v1/workspace, got %q", payload["path"])
	}
	if payload["query"] != "folder=%2Fworkspace-genet" {
		t.Fatalf("expected original query preserved, got %q", payload["query"])
	}
}

func newCodeServerTestHandler() *PodHandler {
	cfg := models.DefaultConfig()
	cfg.Pod.CodeServer.Enabled = true
	return &PodHandler{
		config: cfg,
		log:    zap.NewNop(),
	}
}
