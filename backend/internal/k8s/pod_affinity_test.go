package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBuildPodAffinity_NodeNameOnly(t *testing.T) {
	affinity := buildPodAffinity(nil, "node-a")
	if affinity == nil || affinity.NodeAffinity == nil || affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatalf("expected node affinity with required selector")
	}

	terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != 1 {
		t.Fatalf("expected 1 selector term, got %d", len(terms))
	}
	if !hasHostnameRequirement(terms[0], "node-a") {
		t.Fatalf("expected hostname requirement for node-a")
	}
}

func TestBuildPodAffinity_NodeNameMergedAsAnd(t *testing.T) {
	base := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "accelerator",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"nvidia"},
							},
						},
					},
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "topology.kubernetes.io/zone",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"zone-a"},
							},
						},
					},
				},
			},
		},
	}

	affinity := buildPodAffinity(base, "node-a")
	terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != 2 {
		t.Fatalf("expected 2 selector terms, got %d", len(terms))
	}

	for i, term := range terms {
		if !hasHostnameRequirement(term, "node-a") {
			t.Fatalf("term %d missing hostname requirement", i)
		}
	}
}

func TestBuildPodAffinity_DoesNotMutateBaseConfig(t *testing.T) {
	base := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "accelerator",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"nvidia"},
							},
						},
					},
				},
			},
		},
	}

	affA := buildPodAffinity(base, "node-a")
	affB := buildPodAffinity(base, "node-b")

	baseTerms := base.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(baseTerms) != 1 {
		t.Fatalf("expected base selector terms to remain 1, got %d", len(baseTerms))
	}
	if hasHostnameRequirement(baseTerms[0], "node-a") || hasHostnameRequirement(baseTerms[0], "node-b") {
		t.Fatalf("base affinity should not be mutated with hostname requirement")
	}

	if !hasHostnameRequirement(affA.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0], "node-a") {
		t.Fatalf("expected affA to require node-a")
	}
	if !hasHostnameRequirement(affB.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0], "node-b") {
		t.Fatalf("expected affB to require node-b")
	}
}

func hasHostnameRequirement(term corev1.NodeSelectorTerm, nodeName string) bool {
	for _, expr := range term.MatchExpressions {
		if expr.Key != "kubernetes.io/hostname" || expr.Operator != corev1.NodeSelectorOpIn {
			continue
		}
		for _, value := range expr.Values {
			if value == nodeName {
				return true
			}
		}
	}
	return false
}
