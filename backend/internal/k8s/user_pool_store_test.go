package k8s

import (
	"context"
	"testing"
	"time"

	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestUserPoolBindingStore_EmptyWhenConfigMapMissing(t *testing.T) {
	client := NewClientForTest(fake.NewSimpleClientset(), models.DefaultConfig())

	records, err := client.ListUserPoolBindings(context.Background())
	if err != nil {
		t.Fatalf("ListUserPoolBindings returned error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}

func TestUserPoolBindingStore_UpsertCreatesAndReadsBack(t *testing.T) {
	client := NewClientForTest(fake.NewSimpleClientset(), models.DefaultConfig())
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)

	if err := client.UpsertUserPoolBinding(context.Background(), UserPoolBindingRecord{
		Username:  "alice",
		PoolType:  UserPoolTypeExclusive,
		UpdatedAt: now,
		UpdatedBy: "admin",
	}); err != nil {
		t.Fatalf("UpsertUserPoolBinding returned error: %v", err)
	}

	record, ok, err := client.GetUserPoolBinding(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetUserPoolBinding returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected binding to exist")
	}
	if record.PoolType != UserPoolTypeExclusive {
		t.Fatalf("expected pool type exclusive, got %s", record.PoolType)
	}
	if record.UpdatedBy != "admin" {
		t.Fatalf("expected updatedBy admin, got %s", record.UpdatedBy)
	}
}

func TestUserPoolBindingStore_UpsertOverridesExistingRecord(t *testing.T) {
	cfg := models.DefaultConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.OpenAPI.Namespace}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      UserPoolBindingsConfigMapName,
				Namespace: cfg.OpenAPI.Namespace,
			},
			Data: map[string]string{
				UserPoolBindingsConfigMapDataKey: `[{"username":"alice","poolType":"shared","updatedAt":"2026-03-14T08:00:00Z","updatedBy":"seed"}]`,
			},
		},
	)
	client := NewClientForTest(clientset, cfg)
	now := time.Date(2026, 3, 14, 13, 0, 0, 0, time.UTC)

	if err := client.UpsertUserPoolBinding(context.Background(), UserPoolBindingRecord{
		Username:  "alice",
		PoolType:  UserPoolTypeExclusive,
		UpdatedAt: now,
		UpdatedBy: "admin",
	}); err != nil {
		t.Fatalf("UpsertUserPoolBinding returned error: %v", err)
	}

	record, ok, err := client.GetUserPoolBinding(context.Background(), "alice")
	if err != nil {
		t.Fatalf("GetUserPoolBinding returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected binding to exist")
	}
	if record.PoolType != UserPoolTypeExclusive {
		t.Fatalf("expected pool type exclusive, got %s", record.PoolType)
	}
	if !record.UpdatedAt.Equal(now) {
		t.Fatalf("expected updatedAt %s, got %s", now, record.UpdatedAt)
	}
}
