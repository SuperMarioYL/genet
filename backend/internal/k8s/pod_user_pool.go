package k8s

import (
	"strings"

	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
)

func applyUserPoolSchedulingConstraints(spec *corev1.PodSpec, poolType string, config *models.Config) {
	poolType = NormalizeUserPoolType(poolType)
	if poolType == "" {
		poolType = UserPoolTypeShared
	}

	labelKey := defaultNonSharedLabelKey
	labelValue := defaultNonSharedLabelValue
	taintKey := defaultNonSharedTaintKey
	taintValue := defaultNonSharedTaintValue
	taintEffect := corev1.TaintEffectNoSchedule

	if config != nil {
		if v := strings.TrimSpace(config.GPU.NodePool.NonSharedLabelKey); v != "" {
			labelKey = v
		}
		if v := strings.TrimSpace(config.GPU.NodePool.NonSharedLabelValue); v != "" {
			labelValue = v
		}
		if v := strings.TrimSpace(config.GPU.NodePool.NonSharedTaintKey); v != "" {
			taintKey = v
		}
		if v := strings.TrimSpace(config.GPU.NodePool.NonSharedTaintValue); v != "" {
			taintValue = v
		}
		taintEffect = parseTaintEffect(config.GPU.NodePool.NonSharedTaintEffect)
	}

	if poolType == UserPoolTypeExclusive {
		appendPoolAffinityRequirement(&spec.Affinity, corev1.NodeSelectorRequirement{
			Key:      labelKey,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{labelValue},
		})
		ensureToleration(spec, corev1.Toleration{
			Key:      taintKey,
			Operator: corev1.TolerationOpEqual,
			Value:    taintValue,
			Effect:   taintEffect,
		})
		return
	}

	if labelValue == "" {
		appendPoolAffinityAlternatives(&spec.Affinity, []corev1.NodeSelectorRequirement{
			{
				Key:      labelKey,
				Operator: corev1.NodeSelectorOpDoesNotExist,
			},
		})
		return
	}

	appendPoolAffinityAlternatives(&spec.Affinity, []corev1.NodeSelectorRequirement{
		{
			Key:      labelKey,
			Operator: corev1.NodeSelectorOpDoesNotExist,
		},
		{
			Key:      labelKey,
			Operator: corev1.NodeSelectorOpNotIn,
			Values:   []string{labelValue},
		},
	})
}

func appendPoolAffinityRequirement(affinity **corev1.Affinity, requirement corev1.NodeSelectorRequirement) {
	appendPoolAffinityAlternatives(affinity, []corev1.NodeSelectorRequirement{requirement})
}

func appendPoolAffinityAlternatives(affinity **corev1.Affinity, alternatives []corev1.NodeSelectorRequirement) {
	if *affinity == nil {
		*affinity = &corev1.Affinity{}
	}
	if (*affinity).NodeAffinity == nil {
		(*affinity).NodeAffinity = &corev1.NodeAffinity{}
	}

	required := (*affinity).NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil || len(required.NodeSelectorTerms) == 0 {
		required = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{}},
		}
	}

	nextTerms := make([]corev1.NodeSelectorTerm, 0, len(required.NodeSelectorTerms)*len(alternatives))
	for _, term := range required.NodeSelectorTerms {
		for _, alternative := range alternatives {
			nextTerm := cloneNodeSelectorTerm(term)
			nextTerm.MatchExpressions = append(nextTerm.MatchExpressions, alternative)
			nextTerms = append(nextTerms, nextTerm)
		}
	}

	(*affinity).NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
		NodeSelectorTerms: nextTerms,
	}
}

func ensureToleration(spec *corev1.PodSpec, toleration corev1.Toleration) {
	for _, existing := range spec.Tolerations {
		if existing.Key == toleration.Key &&
			existing.Operator == toleration.Operator &&
			existing.Value == toleration.Value &&
			existing.Effect == toleration.Effect {
			return
		}
	}
	spec.Tolerations = append(spec.Tolerations, toleration)
}

func cloneNodeSelectorTerm(term corev1.NodeSelectorTerm) corev1.NodeSelectorTerm {
	cloned := corev1.NodeSelectorTerm{}
	if len(term.MatchExpressions) > 0 {
		cloned.MatchExpressions = append([]corev1.NodeSelectorRequirement{}, term.MatchExpressions...)
	}
	if len(term.MatchFields) > 0 {
		cloned.MatchFields = append([]corev1.NodeSelectorRequirement{}, term.MatchFields...)
	}
	return cloned
}
