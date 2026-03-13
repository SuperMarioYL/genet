package handlers

import (
	"testing"
	"time"

	"github.com/uc-package/genet/internal/models"
	"github.com/uc-package/genet/internal/prometheus"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildAcceleratorGroupMarksFreshMetrics(t *testing.T) {
	handler := &ClusterHandler{
		config: &models.Config{
			GPU: models.GPUConfig{
				SchedulingMode: "exclusive",
			},
		},
		log: zap.NewNop(),
	}
	accType := models.AcceleratorType{
		Type:         "nvidia",
		Label:        "NVIDIA GPU",
		ResourceName: "nvidia.com/gpu",
	}
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				},
			},
		},
	}
	metrics := &prometheus.AcceleratorMetrics{
		NvidiaGPUs: []prometheus.DeviceMetric{
			{
				Node:        "worker-1",
				DeviceID:    "0",
				Utilization: 12,
				Timestamp:   time.Now().Add(-2 * time.Minute),
			},
		},
	}

	group := handler.buildAcceleratorGroup(accType, nodes, nil, metrics)

	if got := group.Nodes[0].Slots[0].MetricsStatus; got != "fresh" {
		t.Fatalf("expected fresh metrics status, got %q", got)
	}
	if group.Nodes[0].Slots[0].MetricsUpdatedAt == "" {
		t.Fatal("expected metricsUpdatedAt to be populated for fresh metrics")
	}
}

func TestBuildAcceleratorGroupMarksStaleMetrics(t *testing.T) {
	handler := &ClusterHandler{
		config: &models.Config{
			GPU: models.GPUConfig{
				SchedulingMode: "exclusive",
			},
		},
		log: zap.NewNop(),
	}
	accType := models.AcceleratorType{
		Type:         "nvidia",
		Label:        "NVIDIA GPU",
		ResourceName: "nvidia.com/gpu",
	}
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				},
			},
		},
	}
	metrics := &prometheus.AcceleratorMetrics{
		NvidiaGPUs: []prometheus.DeviceMetric{
			{
				Node:        "worker-1",
				DeviceID:    "0",
				Utilization: 12,
				Timestamp:   time.Now().Add(-6 * time.Minute),
			},
		},
	}

	group := handler.buildAcceleratorGroup(accType, nodes, nil, metrics)

	if got := group.Nodes[0].Slots[0].MetricsStatus; got != "stale" {
		t.Fatalf("expected stale metrics status, got %q", got)
	}
}

func TestBuildAcceleratorGroupMarksMissingMetrics(t *testing.T) {
	handler := &ClusterHandler{
		config: &models.Config{
			GPU: models.GPUConfig{
				SchedulingMode: "exclusive",
			},
		},
		log: zap.NewNop(),
	}
	accType := models.AcceleratorType{
		Type:         "nvidia",
		Label:        "NVIDIA GPU",
		ResourceName: "nvidia.com/gpu",
	}
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				},
			},
		},
	}

	group := handler.buildAcceleratorGroup(accType, nodes, nil, &prometheus.AcceleratorMetrics{})

	if got := group.Nodes[0].Slots[0].MetricsStatus; got != "missing" {
		t.Fatalf("expected missing metrics status, got %q", got)
	}
	if group.Nodes[0].Slots[0].MetricsUpdatedAt != "" {
		t.Fatalf("expected empty metricsUpdatedAt for missing metrics, got %q", group.Nodes[0].Slots[0].MetricsUpdatedAt)
	}
}
