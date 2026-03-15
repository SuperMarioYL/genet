package k8s

import (
	"context"
	"testing"

	"github.com/uc-package/genet/internal/models"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListPodsSkipsStatefulSetChildren(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-alice-dev",
				Namespace: "user-alice",
				Labels: map[string]string{
					"genet.io/managed": "true",
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
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: appsv1.SchemeGroupVersion.String(),
						Kind:       "StatefulSet",
						Name:       "sts-alice-train",
					},
				},
			},
		},
	)

	client := NewClientWithClientset(clientset, &models.Config{})
	pods, err := client.ListPods(context.Background(), "user-alice")
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected only standalone pod, got %d", len(pods))
	}
	if pods[0].Name != "pod-alice-dev" {
		t.Fatalf("expected standalone pod to remain, got %s", pods[0].Name)
	}
}

func TestListPodsSkipsDeploymentChildren(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-alice-dev",
				Namespace: "user-alice",
				Labels: map[string]string{
					"genet.io/managed": "true",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deploy-alice-train-7c9d8f8f5b-x2abc",
				Namespace: "user-alice",
				Labels: map[string]string{
					"genet.io/managed":       "true",
					"genet.io/workload-kind": "deployment",
				},
			},
		},
	)

	client := NewClientWithClientset(clientset, &models.Config{})
	pods, err := client.ListPods(context.Background(), "user-alice")
	if err != nil {
		t.Fatalf("list pods: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected only standalone pod, got %d", len(pods))
	}
	if pods[0].Name != "pod-alice-dev" {
		t.Fatalf("expected standalone pod to remain, got %s", pods[0].Name)
	}
}

func TestListStatefulSetsIncludesUnmanagedStatefulSet(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-train",
				Namespace: "user-alice",
			},
		},
	)

	client := NewClientWithClientset(clientset, &models.Config{})
	items, err := client.ListStatefulSets(context.Background(), "user-alice")
	if err != nil {
		t.Fatalf("list statefulsets: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 statefulset, got %d", len(items))
	}
	if items[0].Name != "external-train" {
		t.Fatalf("expected external statefulset to be returned, got %s", items[0].Name)
	}
}

func TestListStatefulSetPodsIncludesOwnerReferencedPods(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-train-0",
				Namespace: "user-alice",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: appsv1.SchemeGroupVersion.String(),
						Kind:       "StatefulSet",
						Name:       "external-train",
					},
				},
			},
		},
	)

	client := NewClientWithClientset(clientset, &models.Config{})
	pods, err := client.ListStatefulSetPods(context.Background(), "user-alice", "external-train")
	if err != nil {
		t.Fatalf("list statefulset pods: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}
	if pods[0].Name != "external-train-0" {
		t.Fatalf("expected owner referenced pod, got %s", pods[0].Name)
	}
}
