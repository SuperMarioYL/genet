package k8s

import (
	"testing"

	"github.com/uc-package/genet/internal/models"
)

func TestBuildConfigMapFromRequestDecodesBinaryData(t *testing.T) {
	req := &models.OpenAPIConfigMapRequest{
		Name:       "cm-demo",
		BinaryData: map[string]string{"app.bin": "aGVsbG8="},
	}

	cm, err := BuildConfigMapFromOpenAPIRequest("user-alice", "alice", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(cm.BinaryData["app.bin"]) != "hello" {
		t.Fatal("expected decoded binaryData")
	}
	if cm.Labels["genet.io/openapi-owner"] != "alice" {
		t.Fatal("expected owner label to be set")
	}
}
