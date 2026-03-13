package genetcli

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadConfigUses0600Permissions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("default config path: %v", err)
	}

	cfg := &Config{
		Server:       "http://localhost:8080",
		Username:     "alice",
		Email:        "alice@example.com",
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Unix(1710000000, 0).UTC(),
	}
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected file mode 0600, got %o", got)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Server != cfg.Server || loaded.Username != cfg.Username || loaded.RefreshToken != cfg.RefreshToken {
		t.Fatalf("loaded config mismatch: %+v", loaded)
	}
}

func TestDefaultConfigPathUsesXDG(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)

	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("default config path: %v", err)
	}

	want := filepath.Join(root, "genet", "config.json")
	if path != want {
		t.Fatalf("expected %q, got %q", want, path)
	}
}
