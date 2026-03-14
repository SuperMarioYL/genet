package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	UserPoolBindingsConfigMapName    = "genet-user-pool-bindings"
	UserPoolBindingsConfigMapDataKey = "records.json"
	UserPoolTypeShared               = "shared"
	UserPoolTypeExclusive            = "exclusive"
)

type UserPoolBindingRecord struct {
	Username  string    `json:"username"`
	PoolType  string    `json:"poolType"`
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy string    `json:"updatedBy"`
}

func NormalizeUserPoolType(poolType string) string {
	switch strings.ToLower(strings.TrimSpace(poolType)) {
	case UserPoolTypeExclusive:
		return UserPoolTypeExclusive
	default:
		return UserPoolTypeShared
	}
}

func IsValidUserPoolType(poolType string) bool {
	return NormalizeUserPoolType(poolType) == strings.TrimSpace(poolType) ||
		strings.TrimSpace(poolType) == UserPoolTypeExclusive ||
		strings.TrimSpace(poolType) == UserPoolTypeShared
}

func (c *Client) ListUserPoolBindings(ctx context.Context) ([]UserPoolBindingRecord, error) {
	cm, err := c.clientset.CoreV1().ConfigMaps(c.getOpenAPINamespace()).Get(ctx, UserPoolBindingsConfigMapName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return []UserPoolBindingRecord{}, nil
		}
		return nil, err
	}

	records, err := decodeUserPoolBindingRecords(cm.Data)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Username < records[j].Username
	})
	return records, nil
}

func (c *Client) GetUserPoolBinding(ctx context.Context, username string) (UserPoolBindingRecord, bool, error) {
	username = strings.TrimSpace(username)
	records, err := c.ListUserPoolBindings(ctx)
	if err != nil {
		return UserPoolBindingRecord{}, false, err
	}
	for _, item := range records {
		if item.Username == username {
			return item, true, nil
		}
	}
	return UserPoolBindingRecord{}, false, nil
}

func (c *Client) UpsertUserPoolBinding(ctx context.Context, rec UserPoolBindingRecord) error {
	rec.Username = strings.TrimSpace(rec.Username)
	rec.UpdatedBy = strings.TrimSpace(rec.UpdatedBy)
	rec.PoolType = NormalizeUserPoolType(rec.PoolType)
	if rec.Username == "" {
		return fmt.Errorf("username is required")
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now().UTC()
	}

	records, err := c.ListUserPoolBindings(ctx)
	if err != nil {
		return err
	}

	found := false
	for i := range records {
		if records[i].Username == rec.Username {
			records[i] = rec
			found = true
			break
		}
	}
	if !found {
		records = append(records, rec)
	}
	return c.saveUserPoolBindings(ctx, records)
}

func (c *Client) saveUserPoolBindings(ctx context.Context, records []UserPoolBindingRecord) error {
	ns := c.getOpenAPINamespace()
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Username < records[j].Username
	})
	dataBytes, err := json.Marshal(records)
	if err != nil {
		return err
	}

	existing, err := c.clientset.CoreV1().ConfigMaps(ns).Get(ctx, UserPoolBindingsConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      UserPoolBindingsConfigMapName,
				Namespace: ns,
				Labels: map[string]string{
					"genet.io/managed": "true",
					"genet.io/type":    "user-pool-bindings",
				},
			},
			Data: map[string]string{
				UserPoolBindingsConfigMapDataKey: string(dataBytes),
			},
		}
		_, err = c.clientset.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}

	if existing.Data == nil {
		existing.Data = map[string]string{}
	}
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels["genet.io/managed"] = "true"
	existing.Labels["genet.io/type"] = "user-pool-bindings"
	existing.Data[UserPoolBindingsConfigMapDataKey] = string(dataBytes)
	_, err = c.clientset.CoreV1().ConfigMaps(ns).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func decodeUserPoolBindingRecords(data map[string]string) ([]UserPoolBindingRecord, error) {
	if len(data) == 0 {
		return []UserPoolBindingRecord{}, nil
	}
	raw := strings.TrimSpace(data[UserPoolBindingsConfigMapDataKey])
	if raw == "" {
		return []UserPoolBindingRecord{}, nil
	}

	var records []UserPoolBindingRecord
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, fmt.Errorf("failed to decode user pool binding records: %w", err)
	}
	return records, nil
}
