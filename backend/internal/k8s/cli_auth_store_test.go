package k8s

import (
	"context"
	"testing"
	"time"

	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCLIAuthRequestStoreCRUD(t *testing.T) {
	client := newTestK8sClient()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	record := models.CLIAuthRequestRecord{
		ID:               "req-1",
		CodeChallenge:    "challenge-1",
		LocalCallbackURL: "http://127.0.0.1:51234/callback",
		State:            "state-1",
		ExpiresAt:        now.Add(5 * time.Minute),
	}

	if err := client.CreateCLIAuthRequest(ctx, record); err != nil {
		t.Fatalf("create auth request: %v", err)
	}

	got, err := client.GetCLIAuthRequest(ctx, record.ID)
	if err != nil {
		t.Fatalf("get auth request: %v", err)
	}
	if got.ID != record.ID {
		t.Fatalf("expected id %q, got %q", record.ID, got.ID)
	}

	record.Username = "alice"
	record.Email = "alice@example.com"
	record.AuthCodeHash = hashCLISecret("plain-auth-code")
	if err := client.UpdateCLIAuthRequest(ctx, record); err != nil {
		t.Fatalf("update auth request: %v", err)
	}

	updated, err := client.GetCLIAuthRequest(ctx, record.ID)
	if err != nil {
		t.Fatalf("get updated auth request: %v", err)
	}
	if updated.Username != "alice" || updated.Email != "alice@example.com" {
		t.Fatalf("expected updated user info, got %+v", updated)
	}
	if updated.AuthCodeHash == "" {
		t.Fatalf("expected auth code hash to be stored")
	}

	if err := client.DeleteCLIAuthRequest(ctx, record.ID); err != nil {
		t.Fatalf("delete auth request: %v", err)
	}

	if _, err := client.GetCLIAuthRequest(ctx, record.ID); err == nil {
		t.Fatalf("expected deleted auth request lookup to fail")
	}
}

func TestConsumeCLIAuthRequest_SingleUse(t *testing.T) {
	client := newTestK8sClient()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	record := models.CLIAuthRequestRecord{
		ID:               "req-2",
		CodeChallenge:    "challenge-2",
		LocalCallbackURL: "http://127.0.0.1:51235/callback",
		State:            "state-2",
		Username:         "alice",
		Email:            "alice@example.com",
		AuthCodeHash:     hashCLISecret("one-time-code"),
		ExpiresAt:        now.Add(5 * time.Minute),
	}

	if err := client.CreateCLIAuthRequest(ctx, record); err != nil {
		t.Fatalf("create auth request: %v", err)
	}

	consumed, err := client.ConsumeCLIAuthRequest(ctx, record.ID, "one-time-code", now)
	if err != nil {
		t.Fatalf("consume auth request: %v", err)
	}
	if consumed.UsedAt == nil {
		t.Fatalf("expected usedAt to be set after consume")
	}

	if _, err := client.ConsumeCLIAuthRequest(ctx, record.ID, "one-time-code", now.Add(time.Second)); err == nil {
		t.Fatalf("expected second consume to fail")
	}
}

func TestCLIRefreshSessionStoreCreateRotateRevoke(t *testing.T) {
	client := newTestK8sClient()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	record := models.CLIRefreshSessionRecord{
		ID:        "sess-1",
		TokenHash: hashCLISecret("refresh-token-1"),
		Username:  "alice",
		Email:     "alice@example.com",
		UserAgent: "genet-cli/test",
		CreatedAt: now,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
	}

	if err := client.CreateCLIRefreshSession(ctx, record); err != nil {
		t.Fatalf("create refresh session: %v", err)
	}

	found, ok, err := client.FindCLIRefreshSessionByPlaintext(ctx, "refresh-token-1")
	if err != nil {
		t.Fatalf("find refresh session: %v", err)
	}
	if !ok {
		t.Fatalf("expected refresh session match")
	}
	if found.ID != record.ID {
		t.Fatalf("expected id %q, got %q", record.ID, found.ID)
	}

	rotated, err := client.RotateCLIRefreshSession(ctx, record.ID, "refresh-token-2", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("rotate refresh session: %v", err)
	}
	if rotated.LastUsedAt == nil {
		t.Fatalf("expected lastUsedAt after rotate")
	}

	if _, ok, err := client.FindCLIRefreshSessionByPlaintext(ctx, "refresh-token-1"); err != nil || ok {
		t.Fatalf("expected old refresh token to stop matching, ok=%v err=%v", ok, err)
	}
	if _, ok, err := client.FindCLIRefreshSessionByPlaintext(ctx, "refresh-token-2"); err != nil || !ok {
		t.Fatalf("expected new refresh token to match, ok=%v err=%v", ok, err)
	}

	if err := client.RevokeCLIRefreshSession(ctx, record.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("revoke refresh session: %v", err)
	}

	if _, ok, err := client.FindCLIRefreshSessionByPlaintext(ctx, "refresh-token-2"); err != nil || ok {
		t.Fatalf("expected revoked refresh session to stop matching, ok=%v err=%v", ok, err)
	}
}

func newTestK8sClient() *Client {
	return &Client{
		clientset: fake.NewSimpleClientset(),
		config:    models.DefaultConfig(),
		log:       logger.Named("k8s-test"),
	}
}

func init() {
	_ = logger.Init(&logger.Config{Level: "error", Format: "console", OutputPath: "stdout"})
	zap.ReplaceGlobals(zap.NewNop())
}
