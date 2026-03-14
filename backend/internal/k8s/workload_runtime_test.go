package k8s

import (
	"context"
	"strings"
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

func TestBuildWorkloadRuntimeInjectsNodeRankForStatefulSet(t *testing.T) {
	client := &Client{
		config: &models.Config{
			Pod: models.PodConfig{
				StartupScript: "echo ready",
			},
		},
		log: logger.Named("k8s-test"),
	}

	spec := &WorkloadRuntimeSpec{
		Name:           "sts-alice-train",
		Username:       "alice",
		Image:          "busybox:latest",
		CPU:            "2",
		Memory:         "4Gi",
		EnableNodeRank: true,
	}

	runtimeSpec, err := client.buildWorkloadRuntime(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, env := range runtimeSpec.Container.Env {
		if env.Name == "NODE_RANK" {
			found = true
			if env.Value != "" {
				t.Fatalf("expected NODE_RANK placeholder to be empty, got %q", env.Value)
			}
		}
	}
	if !found {
		t.Fatal("expected NODE_RANK env var for statefulset runtime")
	}

	if len(runtimeSpec.Container.Command) != 2 || runtimeSpec.Container.Command[0] != "/bin/sh" || runtimeSpec.Container.Command[1] != "-c" {
		t.Fatalf("expected shell startup command, got %#v", runtimeSpec.Container.Command)
	}
	if len(runtimeSpec.Container.Args) != 1 {
		t.Fatalf("expected single shell arg, got %#v", runtimeSpec.Container.Args)
	}
	if want := "export NODE_RANK=\"${POD_NAME##*-}\""; !contains(runtimeSpec.Container.Args[0], want) {
		t.Fatalf("expected startup script to derive NODE_RANK, missing %q in %q", want, runtimeSpec.Container.Args[0])
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
