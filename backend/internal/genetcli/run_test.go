package genetcli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBuildRunPodRequestMapsDevicesAndVolumes(t *testing.T) {
	req, err := buildRunPodRequest("nvidia/cuda:12.0", RunOptions{
		Name:    "train",
		GPUs:    1,
		GPUType: "NVIDIA A100",
		CPU:     "8",
		Memory:  "32Gi",
		ShmSize: "1Gi",
		Node:    "gpu-node-01",
		Devices: "0,2",
		Volumes: []string{"/data:/workspace-genet/data:ro"},
	})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	if req.Image != "nvidia/cuda:12.0" || req.Name != "train" || req.NodeName != "gpu-node-01" {
		t.Fatalf("unexpected request basics: %+v", req)
	}
	if req.GPUCount != 2 {
		t.Fatalf("expected gpuCount 2 from devices, got %d", req.GPUCount)
	}
	if len(req.GPUDevices) != 2 || req.GPUDevices[0] != 0 || req.GPUDevices[1] != 2 {
		t.Fatalf("unexpected gpu devices: %+v", req.GPUDevices)
	}
	if len(req.UserMounts) != 1 || !req.UserMounts[0].ReadOnly {
		t.Fatalf("unexpected mounts: %+v", req.UserMounts)
	}
}

func TestBuildRunPodRequestSupportsCPUOnly(t *testing.T) {
	req, err := buildRunPodRequest("ubuntu:22.04", RunOptions{GPUs: 0})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if req.GPUCount != 0 {
		t.Fatalf("expected cpu-only pod, got gpuCount %d", req.GPUCount)
	}
	if req.GPUType != "" {
		t.Fatalf("expected gpuType omitted for cpu-only pod, got %q", req.GPUType)
	}
}

func TestWaitForPodStopsWhenRunning(t *testing.T) {
	serverHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pods/pod-alice-train" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		serverHits++
		status := "Pending"
		if serverHits >= 2 {
			status = "Running"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "pod-alice-train",
			"name":   "pod-alice-train",
			"status": status,
			"phase":  status,
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, &Config{Server: server.URL, AccessToken: "token"}, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pod, err := waitForPod(ctx, client, "pod-alice-train", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("waitForPod: %v", err)
	}
	if pod.Status != "Running" {
		t.Fatalf("expected running pod, got %+v", pod)
	}
	if serverHits != 2 {
		t.Fatalf("expected 2 polls, got %d", serverHits)
	}
}
