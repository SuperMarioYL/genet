package k8s

import (
	"testing"

	"github.com/uc-package/genet/internal/models"
)

func TestBuildServiceFromRequestUsesTargetPodSelector(t *testing.T) {
	req := &models.OpenAPIServiceRequest{
		Name:          "svc-demo",
		Type:          "ClusterIP",
		TargetPodName: "pod-alice-train",
		Ports: []models.OpenAPIServicePort{
			{Name: "http", Port: 80, TargetPort: "8080"},
		},
	}

	svc, err := BuildServiceFromOpenAPIRequest("user-alice", "alice", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.Spec.Selector["app"] != "pod-alice-train" {
		t.Fatal("expected selector from target pod")
	}
	if svc.Labels["genet.io/openapi-owner"] != "alice" {
		t.Fatal("expected owner label to be set")
	}
}
