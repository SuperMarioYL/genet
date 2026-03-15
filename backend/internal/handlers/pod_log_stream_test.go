package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
)

func TestPodLogStreamEmitsChunkMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	auth.InitAuthMiddleware(cfg)

	var gotOptions k8s.PodLogOptions
	handler := &PodHandler{
		config: cfg,
		log:    zap.NewNop(),
		podLogsUpgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
		streamPodLogsFn: func(_ context.Context, _ string, _ string, options k8s.PodLogOptions) (io.ReadCloser, error) {
			gotOptions = options
			return io.NopCloser(strings.NewReader("2026-03-15T10:00:01Z first line\n2026-03-15T10:00:02Z second line\n")), nil
		},
	}

	router := gin.New()
	router.GET("/api/pods/:id/logs/stream", auth.AuthMiddleware(cfg), handler.PodLogsWebSocket)

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/pods/pod-alice-dev/logs/stream?since=2026-03-15T10:00:00Z"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"X-Auth-Request-User":  []string{"alice"},
		"X-Auth-Request-Email": []string{"alice@example.com"},
	})
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	if !gotOptions.Follow {
		t.Fatalf("expected follow mode to be enabled")
	}
	if !gotOptions.Timestamps {
		t.Fatalf("expected timestamps to be enabled")
	}
	if gotOptions.SinceTime == nil || gotOptions.SinceTime.Format("2006-01-02T15:04:05Z07:00") != "2026-03-15T10:00:00Z" {
		t.Fatalf("expected since time to be forwarded, got %+v", gotOptions.SinceTime)
	}

	var first podLogStreamMessage
	if err := conn.ReadJSON(&first); err != nil {
		t.Fatalf("failed to read first message: %v", err)
	}
	if first.Type != "chunk" {
		t.Fatalf("expected chunk message, got %q", first.Type)
	}
	if first.Content != "first line\n" {
		t.Fatalf("unexpected first content: %q", first.Content)
	}
	if first.Cursor != "2026-03-15T10:00:01Z" {
		t.Fatalf("unexpected first cursor: %q", first.Cursor)
	}

	var second podLogStreamMessage
	if err := conn.ReadJSON(&second); err != nil {
		t.Fatalf("failed to read second message: %v", err)
	}
	if second.Content != "second line\n" {
		t.Fatalf("unexpected second content: %q", second.Content)
	}
}
