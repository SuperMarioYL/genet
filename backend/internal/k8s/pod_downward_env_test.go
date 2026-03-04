package k8s

import "testing"

func TestBuildAutoInjectedDownwardEnvVars(t *testing.T) {
	envs := buildAutoInjectedDownwardEnvVars("workspace")

	expected := []string{
		"NODE_IP",
		"HOST_IP",
		"NODE_NAME",
		"POD_IP",
		"POD_NAME",
		"POD_NAMESPACE",
		"POD_SERVICE_ACCOUNT",
		"POD_UID",
		"CPU_REQUEST",
		"CPU_LIMIT",
		"MEMORY_REQUEST",
		"MEMORY_LIMIT",
	}

	if len(envs) != len(expected) {
		t.Fatalf("expected %d env vars, got %d", len(expected), len(envs))
	}

	for i, name := range expected {
		if envs[i].Name != name {
			t.Fatalf("expected env[%d] name %s, got %s", i, name, envs[i].Name)
		}
		if envs[i].ValueFrom == nil {
			t.Fatalf("expected env[%d] to use valueFrom", i)
		}
	}

	// 资源类变量应绑定到目标容器
	resourceIdx := map[string]int{
		"CPU_REQUEST":    8,
		"CPU_LIMIT":      9,
		"MEMORY_REQUEST": 10,
		"MEMORY_LIMIT":   11,
	}
	for name, idx := range resourceIdx {
		ref := envs[idx].ValueFrom.ResourceFieldRef
		if ref == nil {
			t.Fatalf("%s expected resourceFieldRef", name)
		}
		if ref.ContainerName != "workspace" {
			t.Fatalf("%s expected containerName workspace, got %s", name, ref.ContainerName)
		}
	}
}
