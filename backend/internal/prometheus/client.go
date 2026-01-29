package prometheus

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/uc-package/genet/internal/logger"
	"go.uber.org/zap"
)

// Client Prometheus 客户端
type Client struct {
	api v1.API
	url string
	log *zap.Logger
}

// DeviceMetric 设备指标
type DeviceMetric struct {
	DeviceID    string  `json:"deviceId"`    // 设备编号 "0", "1", "2"...
	Node        string  `json:"node"`        // 节点名
	Pod         string  `json:"pod"`         // Pod 名称
	Namespace   string  `json:"namespace"`   // 命名空间
	Utilization float64 `json:"utilization"` // 利用率 0-100
}

// AcceleratorMetrics 加速卡指标
type AcceleratorMetrics struct {
	NvidiaGPUs []DeviceMetric `json:"nvidiaGPUs"`
	AscendNPUs []DeviceMetric `json:"ascendNPUs"`
}

// NewClient 创建 Prometheus 客户端
func NewClient(url string) (*Client, error) {
	if url == "" {
		return &Client{
			url: "",
			log: logger.Named("prometheus"),
		}, nil
	}

	client, err := api.NewClient(api.Config{
		Address: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus client: %w", err)
	}

	return &Client{
		api: v1.NewAPI(client),
		url: url,
		log: logger.Named("prometheus"),
	}, nil
}

// IsEnabled 检查 Prometheus 是否启用
func (c *Client) IsEnabled() bool {
	return c.url != "" && c.api != nil
}

// QueryAcceleratorMetrics 查询所有加速卡指标
func (c *Client) QueryAcceleratorMetrics(ctx context.Context, acceleratorTypes []AcceleratorTypeConfig) (*AcceleratorMetrics, error) {
	if !c.IsEnabled() {
		return &AcceleratorMetrics{
			NvidiaGPUs: []DeviceMetric{},
			AscendNPUs: []DeviceMetric{},
		}, nil
	}

	result := &AcceleratorMetrics{
		NvidiaGPUs: []DeviceMetric{},
		AscendNPUs: []DeviceMetric{},
	}

	for _, accType := range acceleratorTypes {
		metrics, err := c.queryMetric(ctx, accType.MetricName)
		if err != nil {
			c.log.Warn("Failed to query metric",
				zap.String("metric", accType.MetricName),
				zap.Error(err))
			continue
		}

		switch accType.Type {
		case "nvidia":
			result.NvidiaGPUs = metrics
		case "ascend":
			result.AscendNPUs = metrics
		}
	}

	return result, nil
}

// AcceleratorTypeConfig 加速卡类型配置
type AcceleratorTypeConfig struct {
	Type         string `json:"type"`         // "nvidia" | "ascend"
	Label        string `json:"label"`        // "NVIDIA GPU" | "华为昇腾 NPU"
	ResourceName string `json:"resourceName"` // "nvidia.com/gpu" | "huawei.com/Ascend910"
	MetricName   string `json:"metricName"`   // "DCGM_FI_DEV_GPU_UTIL" | "npu_chip_info_utilization"
}

// queryMetric 查询单个指标
func (c *Client) queryMetric(ctx context.Context, metricName string) ([]DeviceMetric, error) {
	result, warnings, err := c.api.Query(ctx, metricName, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to query prometheus: %w", err)
	}

	if len(warnings) > 0 {
		c.log.Warn("Prometheus query warnings", zap.Strings("warnings", warnings))
	}

	vector, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	metrics := make([]DeviceMetric, 0, len(vector))
	for _, sample := range vector {
		metric := DeviceMetric{
			Utilization: float64(sample.Value),
		}

		// 提取标签
		for name, value := range sample.Metric {
			switch string(name) {
			case "gpu", "id", "device":
				metric.DeviceID = string(value)
			case "node", "instance", "Hostname":
				metric.Node = string(value)
			case "pod", "exported_pod":
				metric.Pod = string(value)
			case "namespace", "exported_namespace":
				metric.Namespace = string(value)
			}
		}

		// 如果没有从标签中获取到设备ID，尝试从 gpu_id 或其他字段
		if metric.DeviceID == "" {
			if gpuID, ok := sample.Metric["gpu_id"]; ok {
				metric.DeviceID = string(gpuID)
			} else if deviceID, ok := sample.Metric["device_id"]; ok {
				metric.DeviceID = string(deviceID)
			}
		}

		// 处理 node 字段：移除端口号
		if metric.Node != "" {
			// 如果 node 包含端口号 (例如 "node-gpu-01:9400")，移除端口部分
			for i, ch := range metric.Node {
				if ch == ':' {
					metric.Node = metric.Node[:i]
					break
				}
			}
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// QueryGPUUtilization 查询 GPU 利用率 (NVIDIA)
func (c *Client) QueryGPUUtilization(ctx context.Context) ([]DeviceMetric, error) {
	return c.queryMetric(ctx, "DCGM_FI_DEV_GPU_UTIL")
}

// QueryNPUUtilization 查询 NPU 利用率 (华为昇腾)
func (c *Client) QueryNPUUtilization(ctx context.Context) ([]DeviceMetric, error) {
	return c.queryMetric(ctx, "npu_chip_info_utilization")
}

// QueryGPUMemory 查询 GPU 显存使用
func (c *Client) QueryGPUMemory(ctx context.Context) (map[string]GPUMemory, error) {
	if !c.IsEnabled() {
		return map[string]GPUMemory{}, nil
	}

	result := make(map[string]GPUMemory)

	// 查询已用显存
	usedMetrics, err := c.queryMetric(ctx, "DCGM_FI_DEV_FB_USED")
	if err != nil {
		c.log.Warn("Failed to query GPU memory used", zap.Error(err))
	} else {
		for _, m := range usedMetrics {
			key := m.Node + "/" + m.DeviceID
			mem := result[key]
			mem.Used = m.Utilization // 这里 Utilization 字段复用存储显存值 (MiB)
			result[key] = mem
		}
	}

	// 查询总显存
	totalMetrics, err := c.queryMetric(ctx, "DCGM_FI_DEV_FB_FREE")
	if err != nil {
		c.log.Warn("Failed to query GPU memory free", zap.Error(err))
	} else {
		for _, m := range totalMetrics {
			key := m.Node + "/" + m.DeviceID
			mem := result[key]
			mem.Free = m.Utilization
			result[key] = mem
		}
	}

	// 计算总量
	for key, mem := range result {
		mem.Total = mem.Used + mem.Free
		result[key] = mem
	}

	return result, nil
}

// GPUMemory GPU 显存信息
type GPUMemory struct {
	Used  float64 `json:"used"`  // 已用 (MiB)
	Free  float64 `json:"free"`  // 空闲 (MiB)
	Total float64 `json:"total"` // 总量 (MiB)
}

// FormatMemory 格式化显存显示
func FormatMemory(mib float64) string {
	if mib >= 1024 {
		return fmt.Sprintf("%.1fGB", mib/1024)
	}
	return fmt.Sprintf("%.0fMB", mib)
}

// ParseDeviceID 解析设备ID为整数
func ParseDeviceID(id string) int {
	if n, err := strconv.Atoi(id); err == nil {
		return n
	}
	return 0
}
