package genetcli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistryHelpersUseExpectedEndpoints(t *testing.T) {
	hits := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path+"?"+r.URL.RawQuery]++
		switch r.URL.Path {
		case "/api/registry/images":
			_ = json.NewEncoder(w).Encode(map[string]any{"images": []any{}})
		case "/api/registry/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"latest"}})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, &Config{Server: server.URL, AccessToken: "token"}, "")
	if _, err := searchRegistryImages(context.Background(), client, "cuda", 20); err != nil {
		t.Fatalf("searchRegistryImages: %v", err)
	}
	if _, err := getRegistryTags(context.Background(), client, "repo/image"); err != nil {
		t.Fatalf("getRegistryTags: %v", err)
	}

	if hits["/api/registry/images?keyword=cuda&limit=20"] != 1 {
		t.Fatalf("expected registry image search hit, got %+v", hits)
	}
	if hits["/api/registry/tags?image=repo%2Fimage"] != 1 {
		t.Fatalf("expected registry tags hit, got %+v", hits)
	}
}
