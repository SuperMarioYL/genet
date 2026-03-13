package genetcli

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/uc-package/genet/internal/models"
)

type CommitStatusResponse struct {
	HasJob      bool   `json:"hasJob"`
	JobName     string `json:"jobName,omitempty"`
	Status      string `json:"status,omitempty"`
	Message     string `json:"message,omitempty"`
	TargetImage string `json:"targetImage,omitempty"`
}

type UserImageListResponse struct {
	Images []models.UserSavedImage `json:"images"`
}

func newCommitCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "commit", Short: "Commit pod images"}
	cmd.AddCommand(&cobra.Command{
		Use:   "<pod-id> <image>",
		Short: "Commit a pod to an image",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			if err := commitImage(cmd.Context(), client, args[0], args[1]); err != nil {
				return err
			}
			return app.print(map[string]string{"message": "commit started"})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status <pod-id>",
		Short: "Get commit status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			var resp CommitStatusResponse
			if err := client.DoJSON(cmd.Context(), "GET", "/api/pods/"+args[0]+"/commit/status", nil, &resp); err != nil {
				return err
			}
			return app.print(resp)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "logs <pod-id>",
		Short: "Get commit logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			var resp map[string]any
			if err := client.DoJSON(cmd.Context(), "GET", "/api/pods/"+args[0]+"/commit/logs", nil, &resp); err != nil {
				return err
			}
			return app.print(resp)
		},
	})
	return cmd
}

func newImageCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "image", Short: "Manage saved images"}
	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List saved images",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			var resp UserImageListResponse
			if err := client.DoJSON(cmd.Context(), "GET", "/api/images", nil, &resp); err != nil {
				return err
			}
			return app.print(resp)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "add <image>",
		Short: "Add a saved image record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			var resp map[string]any
			if err := client.DoJSON(cmd.Context(), "POST", "/api/images", map[string]string{"image": args[0]}, &resp); err != nil {
				return err
			}
			return app.print(resp)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "rm <image>",
		Short: "Delete a saved image record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			var resp map[string]any
			if err := client.DoJSON(cmd.Context(), "DELETE", "/api/images?image="+urlQueryEscape(args[0]), nil, &resp); err != nil {
				return err
			}
			return app.print(resp)
		},
	})
	return cmd
}

func commitImage(ctx context.Context, client *APIClient, podID, imageName string) error {
	return client.DoJSON(ctx, "POST", "/api/pods/"+podID+"/commit", map[string]string{"imageName": imageName}, &map[string]any{})
}
