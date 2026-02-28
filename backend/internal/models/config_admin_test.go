package models

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestLoadConfig_WithAdminUsersAndOpenAPI(t *testing.T) {
	raw := []byte(`
adminUsers:
  - alice
  - bob@example.com
openAPI:
  enabled: true
  namespace: genet-open-api
  apiKeys:
    - legacy-key
`)

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if len(cfg.AdminUsers) != 2 {
		t.Fatalf("expected 2 admin users, got %d", len(cfg.AdminUsers))
	}
	if cfg.AdminUsers[0] != "alice" {
		t.Fatalf("unexpected first admin user: %s", cfg.AdminUsers[0])
	}
	if !cfg.OpenAPI.Enabled {
		t.Fatalf("expected openapi enabled")
	}
	if cfg.OpenAPI.Namespace != "genet-open-api" {
		t.Fatalf("unexpected openapi namespace: %s", cfg.OpenAPI.Namespace)
	}
	if len(cfg.OpenAPI.APIKeys) != 1 || cfg.OpenAPI.APIKeys[0] != "legacy-key" {
		t.Fatalf("unexpected openapi legacy keys: %#v", cfg.OpenAPI.APIKeys)
	}
}
