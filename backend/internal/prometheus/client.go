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
	MemoryUsed  float64 `json:"memoryUsed"`  // 已用显存 (MiB)
	MemoryTotal float64 `json:"memoryTotal"` // 总显存 (MiB)
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
		c.log.Debug("Querying accelerator metrics",
			zap.String("type", accType.Type),
			zap.String("metric", accType.MetricName),
			zap.String("labelConfig.deviceId", accType.MetricLabels.DeviceID),
			zap.String("labelConfig.node", accType.MetricLabels.Node))

		// 查询利用率指标
		metrics, err := c.queryMetricWithLabels(ctx, accType.MetricName, accType.MetricLabels)
		if err != nil {
			c.log.Warn("Failed to query metric",
				zap.String("metric", accType.MetricName),
				zap.Error(err))
			continue
		}

		// 查询显存指标（如果配置了）
		if accType.MemoryUsedMetric != "" {
			memoryUsedMap := c.queryMemoryMetric(ctx, accType.MemoryUsedMetric, accType.MetricLabels)
			memoryFreeMap := map[string]float64{}
			if accType.MemoryTotalMetric != "" {
				memoryFreeMap = c.queryMemoryMetric(ctx, accType.MemoryTotalMetric, accType.MetricLabels)
			}

			// 合并显存数据到利用率指标
			for i := range metrics {
				key := metrics[i].Node + "/" + metrics[i].DeviceID
				if used, ok := memoryUsedMap[key]; ok {
					metrics[i].MemoryUsed = used
				}
				if free, ok := memoryFreeMap[key]; ok {
					// MemoryTotal = Used + Free
					metrics[i].MemoryTotal = metrics[i].MemoryUsed + free
				}
			}
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

// queryMemoryMetric 查询显存指标，返回 node/deviceID -> value 的映射
func (c *Client) queryMemoryMetric(ctx context.Context, metricName string, labelConfig MetricLabelConfig) map[string]float64 {
	result := make(map[string]float64)

	metrics, err := c.queryMetricWithLabels(ctx, metricName, labelConfig)
	if err != nil {
		c.log.Warn("Failed to query memory metric",
			zap.String("metric", metricName),
			zap.Error(err))
		return result
	}

	for _, m := range metrics {
		key := m.Node + "/" + m.DeviceID
		result[key] = m.Utilization // Utilization 字段临时存储显存值
	}

	return result
}

// AcceleratorTypeConfig 加速卡类型配置
type AcceleratorTypeConfig struct {
	Type              string            `json:"type"`              // "nvidia" | "ascend"
	Label             string            `json:"label"`             // "NVIDIA GPU" | "华为昇腾 NPU"
	ResourceName      string            `json:"resourceName"`      // "nvidia.com/gpu" | "huawei.com/Ascend910"
	MetricName        string            `json:"metricName"`        // 利用率指标名
	MemoryUsedMetric  string            `json:"memoryUsedMetric"`  // 显存已用指标名
	MemoryTotalMetric string            `json:"memoryTotalMetric"` // 显存总量指标名
	MetricLabels      MetricLabelConfig `json:"metricLabels"`      // 标签映射配置
}

// MetricLabelConfig 指标标签映射配置
type MetricLabelConfig struct {
	DeviceID  string `json:"deviceId"`  // 设备ID标签名
	Node      string `json:"node"`      // 节点名标签名
	Pod       string `json:"pod"`       // Pod名标签名
	Namespace string `json:"namespace"` // Namespace标签名
}

// queryMetric 查询单个指标
func (c *Client) queryMetric(ctx context.Context, metricName string) ([]DeviceMetric, error) {
	return c.queryMetricWithLabels(ctx, metricName, MetricLabelConfig{})
}

// queryMetricWithLabels 查询指标并使用自定义标签映射
func (c *Client) queryMetricWithLabels(ctx context.Context, metricName string, labelConfig MetricLabelConfig) ([]DeviceMetric, error) {
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

		// 提取标签 - 优先使用自定义标签名，然后是默认标签名
		for name, value := range sample.Metric {
			labelName := string(name)

			// 设备ID
			if labelConfig.DeviceID != "" && labelName == labelConfig.DeviceID {
				metric.DeviceID = string(value)
			} else if labelConfig.DeviceID == "" {
				switch labelName {
				case "gpu", "id", "device", "gpu_id", "device_id", "GPU_I", "minor_number":
					if metric.DeviceID == "" {
						metric.DeviceID = string(value)
					}
				}
			}

			// 节点名 - 优先级: Hostname > node > hostname > kubernetes_node > node_name > instance
			// instance 通常是 IP:port 格式，优先级最低
			if labelConfig.Node != "" && labelName == labelConfig.Node {
				metric.Node = string(value)
			} else if labelConfig.Node == "" {
				switch labelName {
				case "Hostname", "node", "hostname", "kubernetes_node", "node_name":
					// 高优先级标签，直接覆盖
					metric.Node = string(value)
				case "instance":
					// instance 优先级最低，只在没有其他值时使用
					if metric.Node == "" {
						metric.Node = string(value)
					}
				}
			}

			// Pod名
			if labelConfig.Pod != "" && labelName == labelConfig.Pod {
				metric.Pod = string(value)
			} else if labelConfig.Pod == "" {
				switch labelName {
				case "pod", "exported_pod", "pod_name", "kubernetes_pod":
					if metric.Pod == "" {
						metric.Pod = string(value)
					}
				}
			}

			// Namespace
			if labelConfig.Namespace != "" && labelName == labelConfig.Namespace {
				metric.Namespace = string(value)
			} else if labelConfig.Namespace == "" {
				switch labelName {
				case "namespace", "exported_namespace", "kubernetes_namespace":
					if metric.Namespace == "" {
						metric.Namespace = string(value)
					}
				}
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

	c.log.Info("Queried prometheus metric",
		zap.String("metric", metricName),
		zap.Int("count", len(metrics)))

	// 打印详细的指标信息便于调试
	for i, m := range metrics {
		if i < 5 { // 只打印前5个
			c.log.Info("Metric sample",
				zap.String("metric", metricName),
				zap.String("node", m.Node),
				zap.String("deviceId", m.DeviceID),
				zap.String("pod", m.Pod),
				zap.Float64("value", m.Utilization))
		}
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
// 支持格式: "0", "1", "nvidia0", "nvidia1", "device0", "gpu0" 等
func ParseDeviceID(id string) int {
	// 先尝试直接解析数字
	if n, err := strconv.Atoi(id); err == nil {
		return n
	}

	// 尝试提取末尾的数字 (如 "nvidia0" -> 0, "device1" -> 1)
	// 从后往前找连续的数字
	numStart := len(id)
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] >= '0' && id[i] <= '9' {
			numStart = i
		} else {
			break
		}
	}

	if numStart < len(id) {
		if n, err := strconv.Atoi(id[numStart:]); err == nil {
			return n
		}
	}

	return -1 // 返回 -1 表示无法解析，而不是 0（避免和 GPU 0 混淆）
}
