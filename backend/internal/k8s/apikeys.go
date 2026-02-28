package k8s

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// OpenAPIKeysSecretName 存储 OpenAPI key 元数据的 Secret 名称
	OpenAPIKeysSecretName = "genet-openapi-keys"
	// OpenAPIKeysSecretDataKey Secret 中记录列表字段
	OpenAPIKeysSecretDataKey = "records.json"
)

var (
	// ErrOpenAPIKeyNotFound 表示指定 ID 的 key 记录不存在。
	ErrOpenAPIKeyNotFound = errors.New("openapi key not found")
)

// ListOpenAPIKeys 获取管理页维护的 OpenAPI keys。
func (c *Client) ListOpenAPIKeys(ctx context.Context) ([]models.APIKeyRecord, error) {
	ns := c.getOpenAPINamespace()
	secret, err := c.clientset.CoreV1().Secrets(ns).Get(ctx, OpenAPIKeysSecretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return []models.APIKeyRecord{}, nil
		}
		return nil, err
	}
	records, err := decodeOpenAPIKeyRecords(secret.Data)
	if err != nil {
		return nil, err
	}
	sortOpenAPIKeyRecords(records)
	return records, nil
}

// CreateOpenAPIKey 添加 OpenAPI key 记录。
func (c *Client) CreateOpenAPIKey(ctx context.Context, rec models.APIKeyRecord) error {
	records, err := c.ListOpenAPIKeys(ctx)
	if err != nil {
		return err
	}
	for _, item := range records {
		if item.ID == rec.ID {
			return fmt.Errorf("apikey id %q already exists", rec.ID)
		}
	}
	records = append(records, rec)
	return c.saveOpenAPIKeys(ctx, records)
}

// UpdateOpenAPIKey 更新 OpenAPI key 记录。
func (c *Client) UpdateOpenAPIKey(ctx context.Context, rec models.APIKeyRecord) error {
	records, err := c.ListOpenAPIKeys(ctx)
	if err != nil {
		return err
	}
	found := false
	for i := range records {
		if records[i].ID == rec.ID {
			records[i] = rec
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrOpenAPIKeyNotFound, rec.ID)
	}
	return c.saveOpenAPIKeys(ctx, records)
}

// DeleteOpenAPIKey 删除 OpenAPI key 记录。
func (c *Client) DeleteOpenAPIKey(ctx context.Context, id string) error {
	records, err := c.ListOpenAPIKeys(ctx)
	if err != nil {
		return err
	}
	next := make([]models.APIKeyRecord, 0, len(records))
	found := false
	for _, item := range records {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrOpenAPIKeyNotFound, id)
	}
	return c.saveOpenAPIKeys(ctx, next)
}

// FindOpenAPIKeyByPlaintext 通过明文 key 查找记录。
func (c *Client) FindOpenAPIKeyByPlaintext(ctx context.Context, plaintext string) (*models.APIKeyRecord, bool, error) {
	records, err := c.ListOpenAPIKeys(ctx)
	if err != nil {
		return nil, false, err
	}
	rec, ok := findOpenAPIKeyRecordByPlaintext(records, plaintext)
	if !ok {
		return nil, false, nil
	}
	return &rec, true, nil
}

func (c *Client) saveOpenAPIKeys(ctx context.Context, records []models.APIKeyRecord) error {
	ns := c.getOpenAPINamespace()
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}

	sortOpenAPIKeyRecords(records)
	dataBytes, err := json.Marshal(records)
	if err != nil {
		return err
	}

	existing, err := c.clientset.CoreV1().Secrets(ns).Get(ctx, OpenAPIKeysSecretName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      OpenAPIKeysSecretName,
				Namespace: ns,
				Labels: map[string]string{
					"genet.io/managed": "true",
					"genet.io/type":    "openapi-keys",
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				OpenAPIKeysSecretDataKey: dataBytes,
			},
		}
		_, err = c.clientset.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
		return err
	}

	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	existing.Data[OpenAPIKeysSecretDataKey] = dataBytes
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels["genet.io/managed"] = "true"
	existing.Labels["genet.io/type"] = "openapi-keys"
	_, err = c.clientset.CoreV1().Secrets(ns).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func decodeOpenAPIKeyRecords(data map[string][]byte) ([]models.APIKeyRecord, error) {
	if len(data) == 0 {
		return []models.APIKeyRecord{}, nil
	}
	raw := data[OpenAPIKeysSecretDataKey]
	if len(raw) == 0 {
		return []models.APIKeyRecord{}, nil
	}

	var records []models.APIKeyRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("failed to decode openapi key records: %w", err)
	}
	return records, nil
}

func sortOpenAPIKeyRecords(records []models.APIKeyRecord) {
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
}

func (c *Client) getOpenAPINamespace() string {
	if c.config == nil {
		return "genet-open-api"
	}
	ns := strings.TrimSpace(c.config.OpenAPI.Namespace)
	if ns == "" {
		return "genet-open-api"
	}
	return ns
}

func hashOpenAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// HashOpenAPIKey 返回 API Key 的 SHA256 十六进制摘要。
func HashOpenAPIKey(key string) string {
	return hashOpenAPIKey(key)
}

func findOpenAPIKeyRecordByPlaintext(records []models.APIKeyRecord, plaintext string) (models.APIKeyRecord, bool) {
	targetHash := hashOpenAPIKey(plaintext)
	for _, rec := range records {
		if subtle.ConstantTimeCompare([]byte(rec.KeyHash), []byte(targetHash)) == 1 {
			return rec, true
		}
	}
	return models.APIKeyRecord{}, false
}

// IsOpenAPIKeyActive 判断 key 是否可用（启用且未过期）。
func IsOpenAPIKeyActive(rec *models.APIKeyRecord) bool {
	if rec == nil {
		return false
	}
	if !rec.Enabled {
		return false
	}
	if rec.ExpiresAt != nil && rec.ExpiresAt.Before(time.Now()) {
		return false
	}
	return true
}
