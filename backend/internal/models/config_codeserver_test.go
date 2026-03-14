package models

import (
	"strings"
	"testing"
)

func TestDefaultConfigProvidesCodeServerDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Pod.CodeServer.Enabled {
		t.Fatalf("expected code-server disabled by default")
	}
	if cfg.Pod.CodeServer.Port != 13337 {
		t.Fatalf("expected default port 13337, got %d", cfg.Pod.CodeServer.Port)
	}
	if cfg.Pod.CodeServer.WorkspaceDir != DefaultWorkspaceDir {
		t.Fatalf("unexpected workspace dir: %q", cfg.Pod.CodeServer.WorkspaceDir)
	}
	if cfg.Pod.CodeServer.UserDataDir != DefaultCodeServerUserDataDir {
		t.Fatalf("unexpected user data dir: %q", cfg.Pod.CodeServer.UserDataDir)
	}
	if cfg.Pod.CodeServer.ExtensionsDir != DefaultCodeServerExtensionsDir {
		t.Fatalf("unexpected extensions dir: %q", cfg.Pod.CodeServer.ExtensionsDir)
	}
	if cfg.Pod.CodeServer.StartTimeoutSeconds != 20 {
		t.Fatalf("unexpected timeout: %d", cfg.Pod.CodeServer.StartTimeoutSeconds)
	}
	if !strings.Contains(cfg.Pod.StartupScript, "{{.CodeServerScript}}") {
		t.Fatalf("expected startup script to include code-server placeholder")
	}
	if strings.TrimSpace(cfg.Pod.CodeServer.InstallScript) == "" {
		t.Fatalf("expected non-empty code-server install script")
	}
}
