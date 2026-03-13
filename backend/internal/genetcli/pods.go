package genetcli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/uc-package/genet/internal/models"
)

type PodListResponse struct {
	Pods  []PodInfo         `json:"pods"`
	Quota models.QuotaInfo  `json:"quota"`
}

func newPsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List pods",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			var resp PodListResponse
			if err := client.DoJSON(cmd.Context(), "GET", "/api/pods", nil, &resp); err != nil {
				return err
			}
			return app.print(resp)
		},
	}
}

func newPodCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "pod", Short: "Pod detail commands"}
	cmd.AddCommand(&cobra.Command{
		Use:   "get ID",
		Short: "Get a pod",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return podGet(app, cmd, args[0])
		},
	})
	return cmd
}

func newLogsCmd(app *App) *cobra.Command {
	return simpleGETCmd(app, "logs", "Get pod logs", func(id string) string { return "/api/pods/" + id + "/logs" })
}

func newEventsCmd(app *App) *cobra.Command {
	return simpleGETCmd(app, "events", "Get pod events", func(id string) string { return "/api/pods/" + id + "/events" })
}

func newDescribeCmd(app *App) *cobra.Command {
	return simpleGETCmd(app, "describe", "Describe pod", func(id string) string { return "/api/pods/" + id + "/describe" })
}

func newRmCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "rm ID",
		Short: "Delete a pod",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			var resp map[string]any
			if err := client.DoJSON(cmd.Context(), "DELETE", "/api/pods/"+args[0], nil, &resp); err != nil {
				return err
			}
			return app.print(resp)
		},
	}
}

func newProtectCmd(app *App) *cobra.Command {
	var hours int
	cmd := &cobra.Command{
		Use:   "protect ID",
		Short: "Extend pod protection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			path := "/api/pods/" + args[0] + "/extend"
			if hours > 0 {
				path += "?hours=" + url.QueryEscape(fmt.Sprintf("%d", hours))
			}
			var resp map[string]any
			if err := client.DoJSON(cmd.Context(), "POST", path, nil, &resp); err != nil {
				return err
			}
			return app.print(resp)
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 0, "Requested protection duration in hours")
	return cmd
}

func podGet(app *App, cmd *cobra.Command, id string) error {
	client, err := app.apiClient()
	if err != nil {
		return err
	}
	var pod PodInfo
	if err := client.DoJSON(cmd.Context(), "GET", "/api/pods/"+id, nil, &pod); err != nil {
		return err
	}
	return app.print(pod)
}

func simpleGETCmd(app *App, use, short string, pathFn func(string) string) *cobra.Command {
	return &cobra.Command{
		Use:   use + " ID",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			var resp any
			if strings.Contains(use, "logs") || strings.Contains(use, "describe") {
				resp = &map[string]any{}
			} else {
				resp = &map[string]any{}
			}
			if err := client.DoJSON(cmd.Context(), "GET", pathFn(args[0]), nil, resp); err != nil {
				return err
			}
			return app.print(resp)
		},
	}
}
