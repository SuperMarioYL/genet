package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildPodConnectionsIncludesWebShellWhenRunning(t *testing.T) {
	handler := newWebShellTestHandler()
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
	if connections.Apps.WebShellURL != "/pods/pod-alice-dev/webshell" {
		t.Fatalf("unexpected web shell url: %q", connections.Apps.WebShellURL)
	}
	if !connections.Apps.WebShellReady {
		t.Fatalf("expected web shell ready")
	}
	if connections.Apps.WebShellStatus != "enabled" {
		t.Fatalf("expected enabled status, got %q", connections.Apps.WebShellStatus)
	}
}

func TestBuildPodConnectionsMarksWebShellUnavailableWhenPodNotRunning(t *testing.T) {
	handler := newWebShellTestHandler()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-alice-dev"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	connections := handler.buildPodConnections(context.Background(), pod)

	if connections.Apps.WebShellReady {
		t.Fatalf("expected web shell unavailable")
	}
	if connections.Apps.WebShellStatus != "unavailable" {
		t.Fatalf("expected unavailable status, got %q", connections.Apps.WebShellStatus)
	}
}

func TestCreateWebShellSessionCreatesSessionForRunningPod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	auth.InitAuthMiddleware(cfg)

	handler := newWebShellTestHandler()
	handler.config = cfg
	handler.sessions = NewWebShellSessionManager(5 * time.Minute)
	handler.getPodFn = func(context.Context, string, string) (*corev1.Pod, error) {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-alice-dev", Namespace: "user-alice"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "workspace"}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: "10.0.0.8",
			},
		}, nil
	}

	router := gin.New()
	router.POST("/api/pods/:id/webshell/sessions", auth.AuthMiddleware(cfg), handler.CreateWebShellSession)

	reqBody := bytes.NewBufferString(`{"cols":120,"rows":40}`)
	req := httptest.NewRequest(http.MethodPost, "/api/pods/pod-alice-dev/webshell/sessions", reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Request-User", "alice")
	req.Header.Set("X-Auth-Request-Email", "alice@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp WebShellSessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.SessionID == "" {
		t.Fatalf("expected session id")
	}
	if resp.WebSocketURL != "/api/pods/pod-alice-dev/webshell/sessions/"+resp.SessionID+"/ws" {
		t.Fatalf("unexpected websocket url: %q", resp.WebSocketURL)
	}
	if resp.Container != "workspace" {
		t.Fatalf("expected workspace container, got %q", resp.Container)
	}
	if resp.Shell != webShellDisplayShell {
		t.Fatalf("expected %q, got %q", webShellDisplayShell, resp.Shell)
	}
	if _, ok := handler.sessions.Get(resp.SessionID); !ok {
		t.Fatalf("expected session to be stored")
	}
}

func TestShouldFallbackWebShell(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "not found", err: errors.New("exec: \"/bin/bash\": stat /bin/bash: no such file or directory"), want: true},
		{name: "exit 127", err: errors.New("command terminated with exit code 127"), want: true},
		{name: "other", err: errors.New("websocket closed"), want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFallbackWebShell(tc.err); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestCreateWebShellSessionRejectsNonRunningPod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	auth.InitAuthMiddleware(cfg)

	handler := newWebShellTestHandler()
	handler.config = cfg
	handler.sessions = NewWebShellSessionManager(5 * time.Minute)
	handler.getPodFn = func(context.Context, string, string) (*corev1.Pod, error) {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-alice-dev", Namespace: "user-alice"},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
			},
		}, nil
	}

	router := gin.New()
	router.POST("/api/pods/:id/webshell/sessions", auth.AuthMiddleware(cfg), handler.CreateWebShellSession)

	req := httptest.NewRequest(http.MethodPost, "/api/pods/pod-alice-dev/webshell/sessions", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Request-User", "alice")
	req.Header.Set("X-Auth-Request-Email", "alice@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rec.Code)
	}
}

func TestDeleteWebShellSessionRemovesSession(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	auth.InitAuthMiddleware(cfg)

	handler := newWebShellTestHandler()
	handler.config = cfg
	handler.sessions = NewWebShellSessionManager(5 * time.Minute)
	session := handler.sessions.Create(WebShellSessionSpec{
		PodID:          "pod-alice-dev",
		Namespace:      "user-alice",
		UserIdentifier: "alice-alice",
		Container:      "workspace",
		Shell:          "/bin/sh",
		Cols:           120,
		Rows:           40,
	})

	router := gin.New()
	router.DELETE("/api/pods/:id/webshell/sessions/:sessionId", auth.AuthMiddleware(cfg), handler.DeleteWebShellSession)

	req := httptest.NewRequest(http.MethodDelete, "/api/pods/pod-alice-dev/webshell/sessions/"+session.ID, nil)
	req.Header.Set("X-Auth-Request-User", "alice")
	req.Header.Set("X-Auth-Request-Email", "alice@example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if _, ok := handler.sessions.Get(session.ID); ok {
		t.Fatalf("expected session to be deleted")
	}
}

func newWebShellTestHandler() *PodHandler {
	return &PodHandler{
		config:   models.DefaultConfig(),
		log:      zap.NewNop(),
		sessions: NewWebShellSessionManager(5 * time.Minute),
	}
}
