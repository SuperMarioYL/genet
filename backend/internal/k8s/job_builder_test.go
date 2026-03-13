package k8s

import (
	"context"
	"testing"

	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
)

func TestBuildJobFromRequestUsesSharedRuntimeResources(t *testing.T) {
	client := &Client{
		config: &models.Config{
			Pod: models.PodConfig{
				StartupScript: "echo ready",
			},
		},
		log: logger.Named("k8s-test"),
	}

	req := &models.OpenAPIJobRequest{
		Name:    "job-demo",
		Image:   "busybox:latest",
		CPU:     "4",
		Memory:  "8Gi",
		Command: []string{"sh"},
		Args:    []string{"-c", "echo hi"},
		Env: []models.OpenAPIEnvVar{
			{Name: "APP_MODE", Value: "test"},
		},
	}

	job, err := client.BuildJobFromOpenAPIRequest(context.Background(), "user-alice", "alice", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := job.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()
	if got != "4" {
		t.Fatalf("expected cpu request 4, got %s", got)
	}
	if job.Labels["genet.io/openapi-owner"] != "alice" {
		t.Fatal("expected owner label to be set")
	}
	if got := job.Spec.Template.Spec.Containers[0].Env[0].Name; got != "APP_MODE" {
		t.Fatalf("expected first env var APP_MODE, got %s", got)
	}
}
