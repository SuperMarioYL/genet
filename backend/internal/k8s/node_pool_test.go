package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyNodePoolTaint_AddWhenLabeledNonShared(t *testing.T) {
	cfg := resolvedNodePoolConfig{
		enabled:              true,
		nonSharedLabelKey:    "genet.io/node-pool",
		nonSharedLabelValue:  "non-shared",
		nonSharedTaintKey:    "genet.io/non-shared-pool",
		nonSharedTaintValue:  "true",
		nonSharedTaintEffect: corev1.TaintEffectNoSchedule,
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"genet.io/node-pool": "non-shared"},
		},
	}

	changed, action := applyNodePoolTaint(node, cfg)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if action != "added" {
		t.Fatalf("expected action=added, got %s", action)
	}
	if len(node.Spec.Taints) != 1 {
		t.Fatalf("expected 1 taint, got %d", len(node.Spec.Taints))
	}
	taint := node.Spec.Taints[0]
	if taint.Key != "genet.io/non-shared-pool" || taint.Value != "true" || taint.Effect != corev1.TaintEffectNoSchedule {
		t.Fatalf("unexpected taint: %+v", taint)
	}
}

func TestApplyNodePoolTaint_RemoveWhenLabelRemoved(t *testing.T) {
	cfg := resolvedNodePoolConfig{
		enabled:              true,
		nonSharedLabelKey:    "genet.io/node-pool",
		nonSharedLabelValue:  "non-shared",
		nonSharedTaintKey:    "genet.io/non-shared-pool",
		nonSharedTaintValue:  "true",
		nonSharedTaintEffect: corev1.TaintEffectNoSchedule,
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-b",
			Labels: map[string]string{},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "genet.io/non-shared-pool", Value: "true", Effect: corev1.TaintEffectNoSchedule},
				{Key: "node.kubernetes.io/unreachable", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}

	changed, action := applyNodePoolTaint(node, cfg)
	if !changed {
		t.Fatalf("expected changed=true")
	}
	if action != "removed" {
		t.Fatalf("expected action=removed, got %s", action)
	}
	if len(node.Spec.Taints) != 1 {
		t.Fatalf("expected 1 taint remaining, got %d", len(node.Spec.Taints))
	}
	if node.Spec.Taints[0].Key != "node.kubernetes.io/unreachable" {
		t.Fatalf("unexpected remaining taint: %+v", node.Spec.Taints[0])
	}
}

func TestApplyNodePoolTaint_NoChangeWhenAlreadyDesired(t *testing.T) {
	cfg := resolvedNodePoolConfig{
		enabled:              true,
		nonSharedLabelKey:    "genet.io/node-pool",
		nonSharedLabelValue:  "non-shared",
		nonSharedTaintKey:    "genet.io/non-shared-pool",
		nonSharedTaintValue:  "true",
		nonSharedTaintEffect: corev1.TaintEffectNoSchedule,
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-c",
			Labels: map[string]string{"genet.io/node-pool": "non-shared"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "genet.io/non-shared-pool", Value: "true", Effect: corev1.TaintEffectNoSchedule},
			},
		},
	}

	changed, action := applyNodePoolTaint(node, cfg)
	if changed {
		t.Fatalf("expected changed=false")
	}
	if action != "" {
		t.Fatalf("expected empty action, got %s", action)
	}
}
