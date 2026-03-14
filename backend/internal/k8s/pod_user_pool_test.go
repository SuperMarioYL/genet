package k8s

import (
	"testing"

	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
)

func TestApplyUserPoolSchedulingConstraints_ExclusiveAddsAffinityAndToleration(t *testing.T) {
	spec := &corev1.PodSpec{}

	applyUserPoolSchedulingConstraints(spec, UserPoolTypeExclusive, models.DefaultConfig())

	if spec.Affinity == nil || spec.Affinity.NodeAffinity == nil || spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatalf("expected required node affinity")
	}
	terms := spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != 1 {
		t.Fatalf("expected 1 selector term, got %d", len(terms))
	}
	if !hasPoolRequirement(terms[0], defaultNonSharedLabelKey, corev1.NodeSelectorOpIn, defaultNonSharedLabelValue) {
		t.Fatalf("expected exclusive pool requirement, got %#v", terms[0].MatchExpressions)
	}
	if len(spec.Tolerations) != 1 {
		t.Fatalf("expected 1 toleration, got %d", len(spec.Tolerations))
	}
	if spec.Tolerations[0].Key != defaultNonSharedTaintKey {
		t.Fatalf("expected toleration key %s, got %s", defaultNonSharedTaintKey, spec.Tolerations[0].Key)
	}
}

func TestApplyUserPoolSchedulingConstraints_SharedExcludesExclusiveNodes(t *testing.T) {
	spec := &corev1.PodSpec{}

	applyUserPoolSchedulingConstraints(spec, UserPoolTypeShared, models.DefaultConfig())

	terms := spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != 2 {
		t.Fatalf("expected 2 selector terms for shared pool, got %d", len(terms))
	}
	if !hasPoolRequirement(terms[0], defaultNonSharedLabelKey, corev1.NodeSelectorOpDoesNotExist, "") &&
		!hasPoolRequirement(terms[1], defaultNonSharedLabelKey, corev1.NodeSelectorOpDoesNotExist, "") {
		t.Fatalf("expected one shared term to allow unlabeled nodes")
	}
	if !hasPoolRequirement(terms[0], defaultNonSharedLabelKey, corev1.NodeSelectorOpNotIn, defaultNonSharedLabelValue) &&
		!hasPoolRequirement(terms[1], defaultNonSharedLabelKey, corev1.NodeSelectorOpNotIn, defaultNonSharedLabelValue) {
		t.Fatalf("expected one shared term to exclude non-shared label")
	}
	if len(spec.Tolerations) != 0 {
		t.Fatalf("expected no exclusive toleration for shared pool")
	}
}

func hasPoolRequirement(term corev1.NodeSelectorTerm, key string, operator corev1.NodeSelectorOperator, value string) bool {
	for _, expr := range term.MatchExpressions {
		if expr.Key != key || expr.Operator != operator {
			continue
		}
		if value == "" {
			return true
		}
		for _, candidate := range expr.Values {
			if candidate == value {
				return true
			}
		}
	}
	return false
}
