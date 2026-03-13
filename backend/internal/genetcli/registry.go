package genetcli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

type RegistryImageInfo struct {
	Name        string   `json:"name"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description,omitempty"`
}

type SearchImagesResponse struct {
	Images []RegistryImageInfo `json:"images"`
}

type GetImageTagsResponse struct {
	Tags []string `json:"tags"`
}

func newRegistryCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "registry", Short: "Query registry metadata"}
	cmd.AddCommand(&cobra.Command{
		Use:   "search <keyword>",
		Short: "Search registry images",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			resp, err := searchRegistryImages(cmd.Context(), client, args[0], 20)
			if err != nil {
				return err
			}
			return app.print(resp)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "tags <image>",
		Short: "Get image tags",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			resp, err := getRegistryTags(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}
			return app.print(resp)
		},
	})
	return cmd
}

func searchRegistryImages(ctx context.Context, client *APIClient, keyword string, limit int) (*SearchImagesResponse, error) {
	var resp SearchImagesResponse
	if err := client.DoJSON(ctx, "GET", fmt.Sprintf("/api/registry/images?keyword=%s&limit=%d", urlQueryEscape(keyword), limit), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func getRegistryTags(ctx context.Context, client *APIClient, image string) (*GetImageTagsResponse, error) {
	var resp GetImageTagsResponse
	if err := client.DoJSON(ctx, "GET", "/api/registry/tags?image="+urlQueryEscape(image), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
