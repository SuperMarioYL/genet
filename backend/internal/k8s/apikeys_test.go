package k8s

import (
	"testing"

	"github.com/uc-package/genet/internal/models"
)

func TestHashOpenAPIKey_DeterministicAndNonPlaintext(t *testing.T) {
	key := "gk_test_123456"
	h1 := hashOpenAPIKey(key)
	h2 := hashOpenAPIKey(key)

	if h1 == "" {
		t.Fatalf("expected hash not empty")
	}
	if h1 != h2 {
		t.Fatalf("expected deterministic hash")
	}
	if h1 == key {
		t.Fatalf("hash should not equal plaintext")
	}
}

func TestFindOpenAPIKeyRecordByPlaintext_MatchesExpectedRecord(t *testing.T) {
	records := []models.APIKeyRecord{
		{
			ID:      "r1",
			Name:    "read-only",
			KeyHash: hashOpenAPIKey("gk_read"),
			Enabled: true,
		},
		{
			ID:      "r2",
			Name:    "writer",
			KeyHash: hashOpenAPIKey("gk_write"),
			Enabled: true,
		},
	}

	record, ok := findOpenAPIKeyRecordByPlaintext(records, "gk_write")
	if !ok {
		t.Fatalf("expected key to match")
	}
	if record.ID != "r2" {
		t.Fatalf("expected matched id r2, got %s", record.ID)
	}

	if _, ok := findOpenAPIKeyRecordByPlaintext(records, "missing"); ok {
		t.Fatalf("did not expect match for unknown key")
	}
}
