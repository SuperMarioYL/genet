package handlers

import (
	"testing"

	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetNodePoolType_DefaultLabel(t *testing.T) {
	cfg := &models.Config{}
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"genet.io/node-pool": "non-shared",
			},
		},
	}

	poolType := getNodePoolType(node, cfg)
	if poolType != "exclusive" {
		t.Fatalf("expected exclusive, got %s", poolType)
	}
}

func TestGetNodePoolType_CustomLabel(t *testing.T) {
	cfg := &models.Config{
		GPU: models.GPUConfig{
			NodePool: models.NodePoolConfig{
				NonSharedLabelKey:   "pool",
				NonSharedLabelValue: "dedicated",
			},
		},
	}
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"pool": "dedicated",
			},
		},
	}

	poolType := getNodePoolType(node, cfg)
	if poolType != "exclusive" {
		t.Fatalf("expected exclusive, got %s", poolType)
	}
}

func TestGetNodePoolType_SharedWhenNoMatch(t *testing.T) {
	cfg := &models.Config{
		GPU: models.GPUConfig{
			NodePool: models.NodePoolConfig{
				NonSharedLabelKey:   "pool",
				NonSharedLabelValue: "dedicated",
			},
		},
	}
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"pool": "shared",
			},
		},
	}

	poolType := getNodePoolType(node, cfg)
	if poolType != "shared" {
		t.Fatalf("expected shared, got %s", poolType)
	}
}
