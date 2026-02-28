package k8s

import "testing"

func TestBuildShmVolume(t *testing.T) {
	volume, mount, err := buildShmVolume("1Gi")
	if err != nil {
		t.Fatalf("buildShmVolume returned error: %v", err)
	}

	if volume.Name != "genet-shm" {
		t.Fatalf("expected volume name genet-shm, got %s", volume.Name)
	}
	if volume.EmptyDir == nil {
		t.Fatalf("expected EmptyDir volume")
	}
	if volume.EmptyDir.Medium != "Memory" {
		t.Fatalf("expected medium Memory, got %s", volume.EmptyDir.Medium)
	}
	if volume.EmptyDir.SizeLimit == nil {
		t.Fatalf("expected sizeLimit to be set")
	}
	if volume.EmptyDir.SizeLimit.String() != "1Gi" {
		t.Fatalf("expected sizeLimit 1Gi, got %s", volume.EmptyDir.SizeLimit.String())
	}

	if mount.Name != "genet-shm" {
		t.Fatalf("expected mount name genet-shm, got %s", mount.Name)
	}
	if mount.MountPath != "/dev/shm" {
		t.Fatalf("expected mountPath /dev/shm, got %s", mount.MountPath)
	}
}

func TestBuildShmVolume_InvalidSize(t *testing.T) {
	_, _, err := buildShmVolume("1ABC")
	if err == nil {
		t.Fatalf("expected error for invalid shm size")
	}
}
