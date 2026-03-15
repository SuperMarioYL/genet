package cleanup

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCleanupAllPodsDeletesStandalonePod(t *testing.T) {
	config := models.DefaultConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "user-alice",
				Labels: map[string]string{"genet.io/managed": "true"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-alice-dev",
				Namespace: "user-alice",
				Labels:    map[string]string{"genet.io/managed": "true", "genet.io/user": "alice"},
			},
		},
	)

	cleaner := NewPodCleaner(k8s.NewClientWithClientset(clientset, config), config)

	if err := cleaner.CleanupAllPods(); err != nil {
		t.Fatalf("cleanup all pods: %v", err)
	}

	_, err := clientset.CoreV1().Pods("user-alice").Get(t.Context(), "pod-alice-dev", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected standalone pod deleted, got err=%v", err)
	}
}

func TestCleanupAllPodsSuspendsDeploymentWhenCommitSucceeds(t *testing.T) {
	config := models.DefaultConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "user-alice",
				Labels: map[string]string{"genet.io/managed": "true"},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deploy-alice-train",
				Namespace: "user-alice",
				Labels: map[string]string{
					"genet.io/managed":       "true",
					"genet.io/workload-kind": "deployment",
					"genet.io/user":          "alice",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(2),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "workspace", Image: "registry.example.com/alice/train:base"}},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deploy-alice-train-0",
				Namespace: "user-alice",
				Labels: map[string]string{
					"genet.io/managed":       "true",
					"genet.io/workload-kind": "deployment",
					"genet.io/workload-name": "deploy-alice-train",
					"genet.io/user":          "alice",
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	cleaner := NewPodCleaner(k8s.NewClientWithClientset(clientset, config), config)
	cleaner.nowFn = func() time.Time {
		return time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	}
	cleaner.commitWorkloadImageFn = func(_ context.Context, workloadKind, workloadName, namespace, userIdentifier string, pod *corev1.Pod) (string, error) {
		if workloadKind != "deployment" || workloadName != "deploy-alice-train" || namespace != "user-alice" || userIdentifier != "alice" || pod.Name != "deploy-alice-train-0" {
			return "", fmt.Errorf("unexpected suspend target")
		}
		return "registry.example.com/alice/suspend-train:20260315", nil
	}

	if err := cleaner.CleanupAllPods(); err != nil {
		t.Fatalf("cleanup all pods: %v", err)
	}

	deploy, err := clientset.AppsV1().Deployments("user-alice").Get(t.Context(), "deploy-alice-train", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 0 {
		t.Fatalf("expected deployment scaled to 0, got %#v", deploy.Spec.Replicas)
	}
	if deploy.Annotations["genet.io/suspended"] != "true" {
		t.Fatalf("expected suspended=true, got %q", deploy.Annotations["genet.io/suspended"])
	}
}

func TestCleanupAllPodsLeavesStatefulSetRunningWhenCommitFails(t *testing.T) {
	config := models.DefaultConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "user-alice",
				Labels: map[string]string{"genet.io/managed": "true"},
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sts-alice-train",
				Namespace: "user-alice",
				Labels: map[string]string{
					"genet.io/managed":       "true",
					"genet.io/workload-kind": "statefulset",
					"genet.io/user":          "alice",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: int32Ptr(2),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "workspace", Image: "registry.example.com/alice/train:base"}},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sts-alice-train-0",
				Namespace: "user-alice",
				Labels: map[string]string{
					"genet.io/managed":       "true",
					"genet.io/workload-kind": "statefulset",
					"genet.io/workload-name": "sts-alice-train",
					"genet.io/user":          "alice",
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	cleaner := NewPodCleaner(k8s.NewClientWithClientset(clientset, config), config)
	cleaner.commitWorkloadImageFn = func(_ context.Context, workloadKind, workloadName, namespace, userIdentifier string, pod *corev1.Pod) (string, error) {
		return "", fmt.Errorf("commit failed")
	}

	if err := cleaner.CleanupAllPods(); err != nil {
		t.Fatalf("cleanup all pods: %v", err)
	}

	sts, err := clientset.AppsV1().StatefulSets("user-alice").Get(t.Context(), "sts-alice-train", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 2 {
		t.Fatalf("expected statefulset replicas unchanged, got %#v", sts.Spec.Replicas)
	}
	if sts.Annotations["genet.io/suspended"] == "true" {
		t.Fatal("expected statefulset not suspended on commit failure")
	}
}

func int32Ptr(v int32) *int32 {
	return &v
}
