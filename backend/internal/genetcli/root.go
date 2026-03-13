package genetcli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type App struct {
	Server     string
	JSONOutput bool
	ConfigPath string
}

func NewRootCmd() *cobra.Command {
	app := &App{}
	cmd := &cobra.Command{
		Use:   "genet",
		Short: "Genet CLI",
	}
	configPath, _ := DefaultConfigPath()
	app.ConfigPath = configPath
	cmd.PersistentFlags().StringVar(&app.Server, "server", "", "Genet server URL")
	cmd.PersistentFlags().BoolVar(&app.JSONOutput, "json", false, "Output JSON")

	cmd.AddCommand(
		newLoginCmd(app),
		newLogoutCmd(app),
		newWhoamiCmd(app),
		newRunCmd(app),
		newPsCmd(app),
		newPodCmd(app),
		newLogsCmd(app),
		newEventsCmd(app),
		newDescribeCmd(app),
		newRmCmd(app),
		newProtectCmd(app),
		newCommitCmd(app),
		newImageCmd(app),
		newRegistryCmd(app),
		newKubeconfigCmd(app),
	)
	return cmd
}

func newLoginCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Login with browser OAuth",
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.Server == "" {
				return fmt.Errorf("--server is required")
			}
			cfg, err := Login(cmd.Context(), LoginOptions{
				Server:     app.Server,
				ConfigPath: app.ConfigPath,
			})
			if err != nil {
				return err
			}
			return printOutput(app.JSONOutput, map[string]any{
				"server":   cfg.Server,
				"username": cfg.Username,
				"email":    cfg.Email,
			})
		},
	}
}

func newLogoutCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Logout and clear local credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig(app.ConfigPath)
			if err == nil && cfg.RefreshToken != "" && cfg.Server != "" {
				_ = doJSON(context.Background(), &http.Client{}, http.MethodPost, cfg.Server+"/api/cli/auth/logout", map[string]string{
					"refreshToken": cfg.RefreshToken,
				}, nil)
			}
			if err := os.Remove(app.ConfigPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			return printOutput(app.JSONOutput, map[string]string{"message": "logged out"})
		},
	}
}

func newWhoamiCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show current authenticated user",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := LoadConfig(app.ConfigPath)
			if err != nil {
				return err
			}
			return printOutput(app.JSONOutput, map[string]string{
				"server":   cfg.Server,
				"username": cfg.Username,
				"email":    cfg.Email,
			})
		},
	}
}

func printOutput(jsonOutput bool, data any) error {
	output, err := formatOutput(jsonOutput, data)
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, output)
	return nil
}

func formatOutput(jsonOutput bool, data any) (string, error) {
	if jsonOutput {
		buf, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return "", err
		}
		return string(buf) + "\n", nil
	}
	var b strings.Builder
	switch value := data.(type) {
	case map[string]string:
		for k, v := range value {
			fmt.Fprintf(&b, "%s: %s\n", k, v)
		}
	case map[string]any:
		for k, v := range value {
			fmt.Fprintf(&b, "%s: %v\n", k, v)
		}
	default:
		fmt.Fprintf(&b, "%v\n", value)
	}
	return b.String(), nil
}

func (app *App) print(data any) error {
	return printOutput(app.JSONOutput, data)
}

func (app *App) apiClient() (*APIClient, error) {
	cfg, err := LoadConfig(app.ConfigPath)
	if err != nil {
		return nil, err
	}
	server := app.Server
	if server == "" {
		server = cfg.Server
	}
	return NewAPIClient(server, cfg, app.ConfigPath), nil
}

func urlQueryEscape(value string) string {
	return url.QueryEscape(value)
}
