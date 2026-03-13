package k8s

import (
	"context"
	"testing"

	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
)

func TestBuildWorkloadRuntimeIncludesDownwardEnvAndShm(t *testing.T) {
	client := &Client{
		config: &models.Config{
			Pod: models.PodConfig{
				StartupScript: "echo ready",
			},
		},
		log: logger.Named("k8s-test"),
	}

	spec := &WorkloadRuntimeSpec{
		Name:     "demo",
		Username: "alice",
		Image:    "busybox:latest",
		CPU:      "2",
		Memory:   "4Gi",
		ShmSize:  "1Gi",
	}

	runtimeSpec, err := client.buildWorkloadRuntime(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runtimeSpec.Container.Env) == 0 {
		t.Fatal("expected downward env vars")
	}
	if runtimeSpec.Container.Env[0].Name != "NODE_IP" {
		t.Fatalf("expected first env var NODE_IP, got %s", runtimeSpec.Container.Env[0].Name)
	}

	foundShmVolume := false
	foundShmMount := false
	for _, volume := range runtimeSpec.Volumes {
		if volume.Name == "genet-shm" {
			foundShmVolume = true
			break
		}
	}
	for _, mount := range runtimeSpec.Container.VolumeMounts {
		if mount.Name == "genet-shm" && mount.MountPath == "/dev/shm" {
			foundShmMount = true
			break
		}
	}
	if !foundShmVolume {
		t.Fatal("expected shm volume to be added")
	}
	if !foundShmMount {
		t.Fatal("expected shm mount to be added")
	}
}
