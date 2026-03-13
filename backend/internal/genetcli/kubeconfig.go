package genetcli

import (
	"context"
	"os"

	"github.com/spf13/cobra"
)

type KubeconfigResponse struct {
	Kubeconfig string `json:"kubeconfig"`
}

func newKubeconfigCmd(app *App) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "kubeconfig",
		Short: "Get kubeconfig",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Fetch kubeconfig",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			content, err := fetchKubeconfig(cmd.Context(), client)
			if err != nil {
				return err
			}
			if file != "" {
				return writeKubeconfig(file, content)
			}
			return app.print(map[string]string{"kubeconfig": content})
		},
	})
	cmd.PersistentFlags().StringVar(&file, "file", "", "Write kubeconfig to a file")
	return cmd
}

func fetchKubeconfig(ctx context.Context, client *APIClient) (string, error) {
	var resp KubeconfigResponse
	if err := client.DoJSON(ctx, "GET", "/api/kubeconfig", nil, &resp); err != nil {
		return "", err
	}
	return resp.Kubeconfig, nil
}

func writeKubeconfig(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
