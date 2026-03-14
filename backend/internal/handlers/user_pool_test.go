package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestResolveUserPoolType_DefaultsToShared(t *testing.T) {
	client := k8s.NewClientForTest(fake.NewSimpleClientset(), models.DefaultConfig())

	poolType, err := resolveUserPoolType(context.Background(), client, "alice")
	if err != nil {
		t.Fatalf("resolveUserPoolType returned error: %v", err)
	}
	if poolType != k8s.UserPoolTypeShared {
		t.Fatalf("expected shared, got %s", poolType)
	}
}

func TestResolveUserPoolType_UsesStoredBinding(t *testing.T) {
	cfg := models.DefaultConfig()
	client := k8s.NewClientForTest(fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.OpenAPI.Namespace}},
	), cfg)
	if err := client.UpsertUserPoolBinding(context.Background(), k8s.UserPoolBindingRecord{
		Username:  "alice",
		PoolType:  k8s.UserPoolTypeExclusive,
		UpdatedAt: time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC),
		UpdatedBy: "admin",
	}); err != nil {
		t.Fatalf("UpsertUserPoolBinding returned error: %v", err)
	}

	poolType, err := resolveUserPoolType(context.Background(), client, "alice")
	if err != nil {
		t.Fatalf("resolveUserPoolType returned error: %v", err)
	}
	if poolType != k8s.UserPoolTypeExclusive {
		t.Fatalf("expected exclusive, got %s", poolType)
	}
}

func TestValidateRequestedNodePool_RejectsMismatchedPool(t *testing.T) {
	cfg := models.DefaultConfig()
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-a",
			Labels: map[string]string{"genet.io/node-pool": "non-shared"},
		},
	}

	err := validateRequestedNodePool(node, cfg, k8s.UserPoolTypeShared)
	if err == nil {
		t.Fatal("expected mismatch error")
	}
}
