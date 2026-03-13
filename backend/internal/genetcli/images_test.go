package genetcli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommitImageUsesExpectedPathAndPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pods/pod-1/commit" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if req["imageName"] != "registry.local/alice/train:latest" {
			t.Fatalf("unexpected payload: %+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, &Config{Server: server.URL, AccessToken: "token"}, "")
	if err := commitImage(context.Background(), client, "pod-1", "registry.local/alice/train:latest"); err != nil {
		t.Fatalf("commitImage: %v", err)
	}
}
