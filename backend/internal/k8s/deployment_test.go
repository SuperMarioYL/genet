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

func TestListDeploymentsIncludesUnmanagedDeployment(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-deploy",
				Namespace: "user-alice",
			},
		},
	)

	client := NewClientWithClientset(clientset, &models.Config{})
	items, err := client.ListDeployments(context.Background(), "user-alice")
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(items))
	}
	if items[0].Name != "external-deploy" {
		t.Fatalf("expected external deployment to be returned, got %s", items[0].Name)
	}
}

func TestListDeploymentPodsIncludesReplicaSetOwnedPods(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-deploy-6d4c8d5f6f",
				Namespace: "user-alice",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: appsv1.SchemeGroupVersion.String(),
						Kind:       "Deployment",
						Name:       "external-deploy",
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-deploy-6d4c8d5f6f-abcde",
				Namespace: "user-alice",
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: appsv1.SchemeGroupVersion.String(),
						Kind:       "ReplicaSet",
						Name:       "external-deploy-6d4c8d5f6f",
					},
				},
			},
		},
	)

	client := NewClientWithClientset(clientset, &models.Config{})
	pods, err := client.ListDeploymentPods(context.Background(), "user-alice", "external-deploy")
	if err != nil {
		t.Fatalf("list deployment pods: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}
	if pods[0].Name != "external-deploy-6d4c8d5f6f-abcde" {
		t.Fatalf("expected replicaset owned pod, got %s", pods[0].Name)
	}
}
