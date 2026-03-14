package k8s

import (
	"fmt"
	"strings"

	"github.com/uc-package/genet/internal/models"
)

func buildCodeServerStartupScript(cfg models.CodeServerConfig) string {
	if !cfg.Enabled {
		return ""
	}

	workspaceDir := cfg.WorkspaceDir
	if workspaceDir == "" {
		workspaceDir = models.DefaultWorkspaceDir
	}
	userDataDir := cfg.UserDataDir
	if userDataDir == "" {
		userDataDir = models.DefaultCodeServerUserDataDir
	}
	extensionsDir := cfg.ExtensionsDir
	if extensionsDir == "" {
		extensionsDir = models.DefaultCodeServerExtensionsDir
	}
	binDir := strings.TrimRight(userDataDir, "/") + "/bin"
	binaryPath := binDir + "/code-server"
	logFile := strings.TrimRight(userDataDir, "/") + "/code-server.log"
	port := cfg.Port
	if port <= 0 {
		port = 13337
	}
	timeoutSeconds := cfg.StartTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 20
	}
	installScript := strings.TrimSpace(cfg.InstallScript)
	if installScript == "" {
		installScript = "echo \"code-server install skipped: installScript is empty\"\nexit 1"
	}

	return fmt.Sprintf(`
echo "=== Preparing code-server ==="
mkdir -p %s
mkdir -p %s
mkdir -p %s
mkdir -p %s
export PATH=%s:$PATH

if [ -x %s ]; then
  chmod +x %s 2>/dev/null || true
fi

if ! command -v code-server >/dev/null 2>&1; then
  set +e
  (
%s
  )
  INSTALL_EXIT_CODE=$?
  set -e
  if [ "$INSTALL_EXIT_CODE" -ne 0 ]; then
    echo "code-server install failed with exit code $INSTALL_EXIT_CODE; continuing without blocking pod startup"
  fi
fi

if command -v code-server >/dev/null 2>&1; then
  nohup code-server --auth none --bind-addr 0.0.0.0:%d --user-data-dir %s --extensions-dir %s %s > %s 2>&1 &
  CODE_SERVER_READY=0
  CODE_SERVER_WAIT=0
  while [ "$CODE_SERVER_WAIT" -lt %d ]; do
    if command -v curl >/dev/null 2>&1; then
      if curl -fsS http://127.0.0.1:%d/ >/dev/null 2>&1; then
        CODE_SERVER_READY=1
        break
      fi
    elif command -v wget >/dev/null 2>&1; then
      if wget -qO- http://127.0.0.1:%d/ >/dev/null 2>&1; then
        CODE_SERVER_READY=1
        break
      fi
    else
      break
    fi
    sleep 1
    CODE_SERVER_WAIT=$((CODE_SERVER_WAIT + 1))
  done

  if [ "$CODE_SERVER_READY" -eq 1 ]; then
    echo "code-server is ready on port %d"
  else
    echo "code-server did not become ready within %d seconds; continuing without blocking pod startup"
  fi
else
  echo "code-server unavailable after install attempt; continuing without blocking pod startup"
fi
`,
		shellQuote(workspaceDir),
		shellQuote(userDataDir),
		shellQuote(extensionsDir),
		shellQuote(binDir),
		shellQuote(binDir),
		shellQuote(binaryPath),
		shellQuote(binaryPath),
		indentScript(installScript, 4),
		port,
		shellQuote(userDataDir),
		shellQuote(extensionsDir),
		shellQuote(workspaceDir),
		shellQuote(logFile),
		timeoutSeconds,
		port,
		port,
		port,
		timeoutSeconds,
	)
}

func indentScript(script string, spaces int) string {
	if spaces <= 0 {
		return script
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(script, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
