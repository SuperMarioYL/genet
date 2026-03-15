package k8s

import (
	"testing"
	"time"

	"github.com/uc-package/genet/internal/models"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSelectRepresentativePodPrefersRunning(t *testing.T) {
	pod, err := SelectRepresentativePod([]corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "train-2"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "train-1"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "train-0"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
	})
	if err != nil {
		t.Fatalf("select representative pod: %v", err)
	}
	if pod.Name != "train-1" {
		t.Fatalf("expected running pod train-1, got %q", pod.Name)
	}
}

func TestSuspendDeployment(t *testing.T) {
	config := models.DefaultConfig()
	clientset := fake.NewSimpleClientset()
	client := NewClientWithClientset(clientset, config)

	replicas := int32(3)
	_, err := clientset.AppsV1().Deployments("user-alice").Create(t.Context(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deploy-alice-train",
			Namespace: "user-alice",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "workspace", Image: "registry.example.com/alice/train:base"},
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}

	suspendedAt := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	deploy, err := client.SuspendDeployment(t.Context(), "user-alice", "deploy-alice-train", "registry.example.com/alice/suspend-train:20260315", "deploy-alice-train-0", suspendedAt)
	if err != nil {
		t.Fatalf("suspend deployment: %v", err)
	}

	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 0 {
		t.Fatalf("expected deployment scaled to 0, got %#v", deploy.Spec.Replicas)
	}
	if got := deploy.Spec.Template.Spec.Containers[0].Image; got != "registry.example.com/alice/suspend-train:20260315" {
		t.Fatalf("unexpected image: %q", got)
	}
	if deploy.Annotations["genet.io/suspended"] != "true" {
		t.Fatalf("expected suspended=true, got %q", deploy.Annotations["genet.io/suspended"])
	}
	if deploy.Annotations["genet.io/suspended-replicas"] != "3" {
		t.Fatalf("expected suspended replicas 3, got %q", deploy.Annotations["genet.io/suspended-replicas"])
	}
	if deploy.Annotations["genet.io/suspended-source-pod"] != "deploy-alice-train-0" {
		t.Fatalf("unexpected source pod: %q", deploy.Annotations["genet.io/suspended-source-pod"])
	}
}

func TestSuspendStatefulSet(t *testing.T) {
	config := models.DefaultConfig()
	clientset := fake.NewSimpleClientset()
	client := NewClientWithClientset(clientset, config)

	replicas := int32(2)
	_, err := clientset.AppsV1().StatefulSets("user-alice").Create(t.Context(), &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts-alice-train",
			Namespace: "user-alice",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "workspace", Image: "registry.example.com/alice/train:base"},
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create statefulset: %v", err)
	}

	suspendedAt := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	sts, err := client.SuspendStatefulSet(t.Context(), "user-alice", "sts-alice-train", "registry.example.com/alice/suspend-train:20260315", "sts-alice-train-0", suspendedAt)
	if err != nil {
		t.Fatalf("suspend statefulset: %v", err)
	}

	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Fatalf("expected statefulset scaled to 0, got %#v", sts.Spec.Replicas)
	}
	if got := sts.Spec.Template.Spec.Containers[0].Image; got != "registry.example.com/alice/suspend-train:20260315" {
		t.Fatalf("unexpected image: %q", got)
	}
	if sts.Annotations["genet.io/suspended"] != "true" {
		t.Fatalf("expected suspended=true, got %q", sts.Annotations["genet.io/suspended"])
	}
	if sts.Annotations["genet.io/suspended-replicas"] != "2" {
		t.Fatalf("expected suspended replicas 2, got %q", sts.Annotations["genet.io/suspended-replicas"])
	}
	if sts.Annotations["genet.io/suspended-source-pod"] != "sts-alice-train-0" {
		t.Fatalf("unexpected source pod: %q", sts.Annotations["genet.io/suspended-source-pod"])
	}
}
