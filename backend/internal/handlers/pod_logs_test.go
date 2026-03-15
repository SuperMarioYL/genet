package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/k8s"
	"go.uber.org/zap"
)

func TestGetPodLogsSupportsPreviousLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/pods/pod-alice-dev/logs?previous=true", nil)
	c.Params = gin.Params{{Key: "id", Value: "pod-alice-dev"}}
	c.Set("username", "alice")
	c.Set("email", "alice@example.com")

	var gotNamespace string
	var gotPodName string
	var gotOptions k8s.PodLogOptions

	handler := &PodHandler{
		log: zap.NewNop(),
		getPodLogsFn: func(_ context.Context, namespace, name string, options k8s.PodLogOptions) (string, error) {
			gotNamespace = namespace
			gotPodName = name
			gotOptions = options
			return "2026-03-15T10:00:00Z previous logs\n", nil
		},
	}

	handler.GetPodLogs(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if gotNamespace != "user-alice-alice" {
		t.Fatalf("expected namespace user-alice-alice, got %q", gotNamespace)
	}
	if gotPodName != "pod-alice-dev" {
		t.Fatalf("expected pod name pod-alice-dev, got %q", gotPodName)
	}
	if gotOptions.TailLines != 100 {
		t.Fatalf("expected tail lines 100, got %d", gotOptions.TailLines)
	}
	if !gotOptions.Previous {
		t.Fatalf("expected previous logs to be requested")
	}
	if !gotOptions.Timestamps {
		t.Fatalf("expected timestamps to be enabled for cursor extraction")
	}
	if body := recorder.Body.String(); body != "{\"logs\":\"previous logs\\n\",\"cursor\":\"2026-03-15T10:00:00Z\"}" {
		t.Fatalf("unexpected response body: %s", body)
	}
}

func TestGetPodLogsSupportsTailLinesAndSince(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/pods/pod-alice-dev/logs?tailLines=200&since=2026-03-15T10:00:00Z", nil)
	c.Params = gin.Params{{Key: "id", Value: "pod-alice-dev"}}
	c.Set("username", "alice")
	c.Set("email", "alice@example.com")

	var gotOptions k8s.PodLogOptions

	handler := &PodHandler{
		log: zap.NewNop(),
		getPodLogsFn: func(_ context.Context, _ string, _ string, options k8s.PodLogOptions) (string, error) {
			gotOptions = options
			return "2026-03-15T10:00:01Z next line\n", nil
		},
	}

	handler.GetPodLogs(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if gotOptions.TailLines != 200 {
		t.Fatalf("expected tail lines 200, got %d", gotOptions.TailLines)
	}
	if gotOptions.SinceTime == nil || gotOptions.SinceTime.Format("2006-01-02T15:04:05Z07:00") != "2026-03-15T10:00:00Z" {
		t.Fatalf("expected since time to be parsed, got %+v", gotOptions.SinceTime)
	}
	if body := recorder.Body.String(); body != "{\"logs\":\"next line\\n\",\"cursor\":\"2026-03-15T10:00:01Z\"}" {
		t.Fatalf("unexpected response body: %s", body)
	}
}
