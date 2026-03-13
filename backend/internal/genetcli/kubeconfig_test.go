package genetcli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"net/http"
	"net/http/httptest"
)

func TestKubeconfigGetSupportsStdoutAndFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/kubeconfig" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"kubeconfig": "apiVersion: v1\nclusters: []\n",
			"namespace":  "user-alice",
			"clusterName": "test-cluster",
			"mode":        "oidc",
			"instructions": map[string]any{"usage": []string{"test"}},
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, &Config{Server: server.URL, AccessToken: "token"}, "")
	content, err := fetchKubeconfig(context.Background(), client)
	if err != nil {
		t.Fatalf("fetchKubeconfig: %v", err)
	}
	if content != "apiVersion: v1\nclusters: []\n" {
		t.Fatalf("unexpected kubeconfig content %q", content)
	}

	target := filepath.Join(t.TempDir(), "config")
	if err := writeKubeconfig(target, content); err != nil {
		t.Fatalf("writeKubeconfig: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read kubeconfig: %v", err)
	}
	if string(data) != content {
		t.Fatalf("unexpected kubeconfig file content %q", string(data))
	}
}
