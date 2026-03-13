package genetcli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/uc-package/genet/internal/models"
)

type RunOptions struct {
	Name    string
	GPUs    int
	GPUType string
	CPU     string
	Memory  string
	ShmSize string
	Node    string
	Devices string
	Volumes []string
	Wait    bool
}

type CreatePodResponse struct {
	Message string `json:"message"`
	ID      string `json:"id"`
	Name    string `json:"name"`
}

type PodInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	Phase          string `json:"phase"`
	Namespace      string `json:"namespace,omitempty"`
	Image          string `json:"image,omitempty"`
	GPUType        string `json:"gpuType,omitempty"`
	GPUCount       int    `json:"gpuCount,omitempty"`
	CPU            string `json:"cpu,omitempty"`
	Memory         string `json:"memory,omitempty"`
	NodeIP         string `json:"nodeIP,omitempty"`
	ProtectedUntil string `json:"protectedUntil,omitempty"`
}

func newRunCmd(app *App) *cobra.Command {
	opts := RunOptions{}
	cmd := &cobra.Command{
		Use:   "run IMAGE",
		Short: "Create a pod",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := app.apiClient()
			if err != nil {
				return err
			}
			req, err := buildRunPodRequest(args[0], opts)
			if err != nil {
				return err
			}
			var created CreatePodResponse
			if err := client.DoJSON(cmd.Context(), "POST", "/api/pods", req, &created); err != nil {
				return err
			}
			if opts.Wait {
				pod, err := waitForPod(cmd.Context(), client, created.ID, 2*time.Second)
				if err != nil {
					return err
				}
				return app.print(pod)
			}
			return app.print(created)
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "Pod name suffix")
	cmd.Flags().IntVar(&opts.GPUs, "gpus", 1, "GPU count")
	cmd.Flags().StringVar(&opts.GPUType, "gpu-type", "", "GPU type")
	cmd.Flags().StringVar(&opts.CPU, "cpu", "", "CPU request")
	cmd.Flags().StringVar(&opts.Memory, "memory", "", "Memory request")
	cmd.Flags().StringVar(&opts.ShmSize, "shm-size", "", "Shared memory size")
	cmd.Flags().StringVar(&opts.Node, "node", "", "Node name")
	cmd.Flags().StringVar(&opts.Devices, "device", "", "GPU device list, e.g. 0,1")
	cmd.Flags().StringArrayVarP(&opts.Volumes, "volume", "v", nil, "Volume mounts host:container[:ro|rw]")
	cmd.Flags().BoolVar(&opts.Wait, "wait", false, "Wait until pod is running")
	return cmd
}

func buildRunPodRequest(image string, opts RunOptions) (models.PodRequest, error) {
	req := models.PodRequest{
		Image:    image,
		GPUType:  opts.GPUType,
		GPUCount: opts.GPUs,
		CPU:      opts.CPU,
		Memory:   opts.Memory,
		ShmSize:  opts.ShmSize,
		NodeName: opts.Node,
		Name:     opts.Name,
	}
	if strings.TrimSpace(opts.Devices) != "" {
		devices, err := parseDeviceList(opts.Devices)
		if err != nil {
			return models.PodRequest{}, err
		}
		req.GPUDevices = devices
		req.GPUCount = len(devices)
	}
	if req.GPUCount == 0 {
		req.GPUType = ""
	}
	for _, spec := range opts.Volumes {
		mount, err := parseVolumeSpec(spec)
		if err != nil {
			return models.PodRequest{}, err
		}
		req.UserMounts = append(req.UserMounts, mount)
	}
	return req, nil
}

func parseDeviceList(spec string) ([]int, error) {
	parts := strings.Split(spec, ",")
	devices := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid device %q", part)
		}
		devices = append(devices, value)
	}
	return devices, nil
}

func parseVolumeSpec(spec string) (models.UserMount, error) {
	parts := strings.Split(spec, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return models.UserMount{}, fmt.Errorf("invalid volume spec %q", spec)
	}
	mount := models.UserMount{
		HostPath:  parts[0],
		MountPath: parts[1],
	}
	if len(parts) == 3 {
		switch parts[2] {
		case "ro":
			mount.ReadOnly = true
		case "rw", "":
		default:
			return models.UserMount{}, fmt.Errorf("invalid volume mode %q", parts[2])
		}
	}
	return mount, nil
}

func waitForPod(ctx context.Context, client *APIClient, id string, interval time.Duration) (*PodInfo, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		var pod PodInfo
		if err := client.DoJSON(ctx, "GET", "/api/pods/"+id, nil, &pod); err != nil {
			return nil, err
		}
		switch pod.Status {
		case "Running":
			return &pod, nil
		case "Failed", "Error", "Succeeded":
			return &pod, fmt.Errorf("pod reached terminal state %s", pod.Status)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
