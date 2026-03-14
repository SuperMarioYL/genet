package k8s

import (
	"strings"
	"testing"

	"github.com/uc-package/genet/internal/models"
)

func TestBuildCodeServerStartupScriptReturnsEmptyWhenDisabled(t *testing.T) {
	if got := buildCodeServerStartupScript(models.CodeServerConfig{}); got != "" {
		t.Fatalf("expected empty script when disabled, got %q", got)
	}
}

func TestBuildCodeServerStartupScriptIncludesExpectedCommand(t *testing.T) {
	script := buildCodeServerStartupScript(models.CodeServerConfig{
		Enabled:             true,
		Port:                13337,
		WorkspaceDir:        models.DefaultWorkspaceDir,
		UserDataDir:         models.DefaultCodeServerUserDataDir,
		ExtensionsDir:       models.DefaultCodeServerExtensionsDir,
		InstallScript:       "echo install code-server",
		StartTimeoutSeconds: 20,
	})

	expectedSnippets := []string{
		"echo install code-server",
		"mkdir -p '" + models.DefaultCodeServerUserDataDir + "'",
		"--bind-addr 0.0.0.0:13337",
		"--user-data-dir '" + models.DefaultCodeServerUserDataDir + "'",
		"--extensions-dir '" + models.DefaultCodeServerExtensionsDir + "'",
		"'" + models.DefaultWorkspaceDir + "' > '" + models.DefaultCodeServerUserDataDir + "/code-server.log' 2>&1 &",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected script to contain %q, got:\n%s", snippet, script)
		}
	}
}

func TestBuildCodeServerStartupScriptDoesNotAbortPodStartupOnInstallFailure(t *testing.T) {
	script := buildCodeServerStartupScript(models.CodeServerConfig{
		Enabled:       true,
		Port:          13337,
		WorkspaceDir:  models.DefaultWorkspaceDir,
		UserDataDir:   models.DefaultCodeServerUserDataDir,
		ExtensionsDir: models.DefaultCodeServerExtensionsDir,
		InstallScript: "exit 1",
	})

	expectedSnippets := []string{
		"set +e",
		"INSTALL_EXIT_CODE=$?",
		"continuing without blocking pod startup",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(script, snippet) {
			t.Fatalf("expected script to contain %q, got:\n%s", snippet, script)
		}
	}
}
