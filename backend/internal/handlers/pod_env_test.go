package handlers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestExtractAutoInjectedEnvVarNames(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "workspace",
					Env: []corev1.EnvVar{
						{Name: "CUSTOM_ENV"},
						{Name: "POD_NAME"},
						{Name: "MEMORY_LIMIT"},
						{Name: "CPU_REQUEST"},
					},
				},
			},
		},
	}

	got := extractAutoInjectedEnvVarNames(pod)
	expected := []string{"POD_NAME", "CPU_REQUEST", "MEMORY_LIMIT"}

	if len(got) != len(expected) {
		t.Fatalf("expected %d env names, got %d (%v)", len(expected), len(got), got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("expected env[%d]=%s, got %s", i, expected[i], got[i])
		}
	}
}
