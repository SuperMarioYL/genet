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
	CLIAuthRequestsSecretName    = "genet-cli-auth-requests"
	CLIRefreshSessionsSecretName = "genet-cli-refresh-sessions"
	CLIAuthStoreSecretDataKey    = "records.json"
)

var (
	ErrCLIAuthRequestNotFound    = errors.New("cli auth request not found")
	ErrCLIAuthRequestUsed        = errors.New("cli auth request already used")
	ErrCLIAuthCodeMismatch       = errors.New("cli auth code mismatch")
	ErrCLIRefreshSessionNotFound = errors.New("cli refresh session not found")
)

func (c *Client) CreateCLIAuthRequest(ctx context.Context, rec models.CLIAuthRequestRecord) error {
	records, err := c.listCLIAuthRequests(ctx)
	if err != nil {
		return err
	}
	for _, item := range records {
		if item.ID == rec.ID {
			return fmt.Errorf("cli auth request %q already exists", rec.ID)
		}
	}
	records = append(records, rec)
	return c.saveCLIAuthRequests(ctx, records)
}

func (c *Client) GetCLIAuthRequest(ctx context.Context, id string) (*models.CLIAuthRequestRecord, error) {
	records, err := c.listCLIAuthRequests(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range records {
		if item.ID == id {
			record := item
			return &record, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrCLIAuthRequestNotFound, id)
}

func (c *Client) UpdateCLIAuthRequest(ctx context.Context, rec models.CLIAuthRequestRecord) error {
	records, err := c.listCLIAuthRequests(ctx)
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
		return fmt.Errorf("%w: %s", ErrCLIAuthRequestNotFound, rec.ID)
	}
	return c.saveCLIAuthRequests(ctx, records)
}

func (c *Client) DeleteCLIAuthRequest(ctx context.Context, id string) error {
	records, err := c.listCLIAuthRequests(ctx)
	if err != nil {
		return err
	}
	next := make([]models.CLIAuthRequestRecord, 0, len(records))
	found := false
	for _, item := range records {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrCLIAuthRequestNotFound, id)
	}
	return c.saveCLIAuthRequests(ctx, next)
}

func (c *Client) ConsumeCLIAuthRequest(ctx context.Context, id, plaintextCode string, usedAt time.Time) (*models.CLIAuthRequestRecord, error) {
	records, err := c.listCLIAuthRequests(ctx)
	if err != nil {
		return nil, err
	}
	target := -1
	for i := range records {
		if records[i].ID == id {
			target = i
			break
		}
	}
	if target < 0 {
		return nil, fmt.Errorf("%w: %s", ErrCLIAuthRequestNotFound, id)
	}
	record := records[target]
	if record.UsedAt != nil {
		return nil, fmt.Errorf("%w: %s", ErrCLIAuthRequestUsed, id)
	}
	if subtle.ConstantTimeCompare([]byte(record.AuthCodeHash), []byte(hashCLISecret(plaintextCode))) != 1 {
		return nil, ErrCLIAuthCodeMismatch
	}
	record.UsedAt = &usedAt
	records[target] = record
	if err := c.saveCLIAuthRequests(ctx, records); err != nil {
		return nil, err
	}
	return &record, nil
}

func (c *Client) CreateCLIRefreshSession(ctx context.Context, rec models.CLIRefreshSessionRecord) error {
	records, err := c.listCLIRefreshSessions(ctx)
	if err != nil {
		return err
	}
	for _, item := range records {
		if item.ID == rec.ID {
			return fmt.Errorf("cli refresh session %q already exists", rec.ID)
		}
	}
	records = append(records, rec)
	return c.saveCLIRefreshSessions(ctx, records)
}

func (c *Client) FindCLIRefreshSessionByPlaintext(ctx context.Context, plaintext string) (*models.CLIRefreshSessionRecord, bool, error) {
	records, err := c.listCLIRefreshSessions(ctx)
	if err != nil {
		return nil, false, err
	}
	targetHash := hashCLISecret(plaintext)
	now := time.Now().UTC()
	for _, item := range records {
		if item.RevokedAt != nil || item.ExpiresAt.Before(now) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(item.TokenHash), []byte(targetHash)) == 1 {
			record := item
			return &record, true, nil
		}
	}
	return nil, false, nil
}

func (c *Client) RotateCLIRefreshSession(ctx context.Context, id, nextPlaintext string, usedAt time.Time) (*models.CLIRefreshSessionRecord, error) {
	records, err := c.listCLIRefreshSessions(ctx)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].ID != id {
			continue
		}
		records[i].TokenHash = hashCLISecret(nextPlaintext)
		records[i].LastUsedAt = &usedAt
		if err := c.saveCLIRefreshSessions(ctx, records); err != nil {
			return nil, err
		}
		record := records[i]
		return &record, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrCLIRefreshSessionNotFound, id)
}

func (c *Client) RevokeCLIRefreshSession(ctx context.Context, id string, revokedAt time.Time) error {
	records, err := c.listCLIRefreshSessions(ctx)
	if err != nil {
		return err
	}
	for i := range records {
		if records[i].ID != id {
			continue
		}
		records[i].RevokedAt = &revokedAt
		return c.saveCLIRefreshSessions(ctx, records)
	}
	return fmt.Errorf("%w: %s", ErrCLIRefreshSessionNotFound, id)
}

func (c *Client) listCLIAuthRequests(ctx context.Context) ([]models.CLIAuthRequestRecord, error) {
	secret, err := c.clientset.CoreV1().Secrets(c.getCLIAuthNamespace()).Get(ctx, CLIAuthRequestsSecretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return []models.CLIAuthRequestRecord{}, nil
		}
		return nil, err
	}
	var records []models.CLIAuthRequestRecord
	if err := decodeCLIAuthRecords(secret.Data, &records); err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ExpiresAt.Before(records[j].ExpiresAt) })
	return records, nil
}

func (c *Client) saveCLIAuthRequests(ctx context.Context, records []models.CLIAuthRequestRecord) error {
	return c.saveCLIAuthSecret(ctx, CLIAuthRequestsSecretName, "cli-auth-requests", records)
}

func (c *Client) listCLIRefreshSessions(ctx context.Context) ([]models.CLIRefreshSessionRecord, error) {
	secret, err := c.clientset.CoreV1().Secrets(c.getCLIAuthNamespace()).Get(ctx, CLIRefreshSessionsSecretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return []models.CLIRefreshSessionRecord{}, nil
		}
		return nil, err
	}
	var records []models.CLIRefreshSessionRecord
	if err := decodeCLIAuthRecords(secret.Data, &records); err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.Before(records[j].CreatedAt) })
	return records, nil
}

func (c *Client) saveCLIRefreshSessions(ctx context.Context, records []models.CLIRefreshSessionRecord) error {
	return c.saveCLIAuthSecret(ctx, CLIRefreshSessionsSecretName, "cli-refresh-sessions", records)
}

func (c *Client) saveCLIAuthSecret(ctx context.Context, name, recordType string, records interface{}) error {
	ns := c.getCLIAuthNamespace()
	if err := c.EnsureNamespace(ctx, ns); err != nil {
		return err
	}
	dataBytes, err := json.Marshal(records)
	if err != nil {
		return err
	}
	existing, err := c.clientset.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return err
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels: map[string]string{
					"genet.io/managed": "true",
					"genet.io/type":    recordType,
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				CLIAuthStoreSecretDataKey: dataBytes,
			},
		}
		_, err = c.clientset.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
		return err
	}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	existing.Data[CLIAuthStoreSecretDataKey] = dataBytes
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels["genet.io/managed"] = "true"
	existing.Labels["genet.io/type"] = recordType
	_, err = c.clientset.CoreV1().Secrets(ns).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func decodeCLIAuthRecords(data map[string][]byte, out interface{}) error {
	if len(data) == 0 {
		return nil
	}
	raw := data[CLIAuthStoreSecretDataKey]
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("failed to decode cli auth records: %w", err)
	}
	return nil
}

func (c *Client) getCLIAuthNamespace() string {
	return c.getOpenAPINamespace()
}

func hashCLISecret(secret string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(secret)))
	return hex.EncodeToString(sum[:])
}

func HashCLISecretForTest(secret string) string {
	return hashCLISecret(secret)
}
