package handlers

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"github.com/uc-package/genet/internal/prometheus"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterHandler 集群相关处理器
type ClusterHandler struct {
	k8sClient  *k8s.Client
	promClient *prometheus.Client
	config     *models.Config
	log        *zap.Logger
}

// NewClusterHandler 创建集群处理器
func NewClusterHandler(k8sClient *k8s.Client, promClient *prometheus.Client, config *models.Config) *ClusterHandler {
	return &ClusterHandler{
		k8sClient:  k8sClient,
		promClient: promClient,
		config:     config,
		log:        logger.Named("cluster"),
	}
}

// AcceleratorGroup 加速卡分组
type AcceleratorGroup struct {
	Type         string     `json:"type"`         // "nvidia" | "ascend"
	Label        string     `json:"label"`        // "NVIDIA GPU" | "华为昇腾 NPU"
	ResourceName string     `json:"resourceName"` // "nvidia.com/gpu" | "huawei.com/Ascend910"
	Nodes        []NodeInfo `json:"nodes"`        // 节点列表
	TotalDevices int        `json:"totalDevices"` // 总设备数
	UsedDevices  int        `json:"usedDevices"`  // 已用设备数
}

// NodeInfo 节点信息
type NodeInfo struct {
	NodeName            string       `json:"nodeName"`            // 节点名
	NodeIP              string       `json:"nodeIP"`              // 节点 IP
	DeviceType          string       `json:"deviceType"`          // 设备型号
	TotalDevices        int          `json:"totalDevices"`        // 总设备数
	UsedDevices         int          `json:"usedDevices"`         // 已用设备数
	Slots               []DeviceSlot `json:"slots"`               // 设备槽位
	TimeSharingEnabled  bool         `json:"timeSharingEnabled"`  // 是否支持时分复用
	TimeSharingReplicas int          `json:"timeSharingReplicas"` // 每卡可共享数（如 4）
}

// DeviceSlot 设备槽位
type DeviceSlot struct {
	Index       int      `json:"index"`       // 设备编号
	Status      string   `json:"status"`      // "free" | "used"
	Utilization float64  `json:"utilization"` // 利用率 0-100
	Pod         *PodInfo `json:"pod"`         // 占用的 Pod 信息
}

// PodInfo Pod 简要信息
type PodInfo struct {
	Name      string `json:"name"`      // Pod 名称
	Namespace string `json:"namespace"` // 命名空间
	User      string `json:"user"`      // 用户名
	Email     string `json:"email"`     // 用户邮箱
	GPUCount  int    `json:"gpuCount"`  // GPU 数量
	StartTime string `json:"startTime"` // 启动时间
}

// GPUOverviewResponse GPU 概览响应
type GPUOverviewResponse struct {
	AcceleratorGroups []AcceleratorGroup `json:"acceleratorGroups"` // 加速卡分组
	Summary           Summary            `json:"summary"`           // 汇总信息
	UpdatedAt         time.Time          `json:"updatedAt"`         // 更新时间
	PrometheusEnabled bool               `json:"prometheusEnabled"` // Prometheus 是否已配置
}

// Summary 汇总信息
type Summary struct {
	TotalDevices int               `json:"totalDevices"` // 总设备数
	UsedDevices  int               `json:"usedDevices"`  // 已用设备数
	ByType       map[string]TypeSummary `json:"byType"`  // 按类型汇总
}

// TypeSummary 类型汇总
type TypeSummary struct {
	Total int `json:"total"` // 总数
	Used  int `json:"used"`  // 已用
}

// GetGPUOverview 获取 GPU 概览
func (h *ClusterHandler) GetGPUOverview(c *gin.Context) {
	ctx := context.Background()

	// 获取所有节点
	nodes, err := h.k8sClient.GetClientset().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		h.log.Error("Failed to list nodes", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取节点列表失败"})
		return
	}

	// 获取所有 Pod（用于匹配 GPU 使用者）
	pods, err := h.k8sClient.GetClientset().CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		h.log.Error("Failed to list pods", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 Pod 列表失败"})
		return
	}

	// 从配置推导出加速卡类型
	acceleratorTypes := h.config.GetAcceleratorTypes()

	// 从 Prometheus 获取 GPU 指标
	var acceleratorMetrics *prometheus.AcceleratorMetrics
	if h.promClient != nil && h.promClient.IsEnabled() {
		promTypes := make([]prometheus.AcceleratorTypeConfig, len(acceleratorTypes))
		for i, t := range acceleratorTypes {
			promTypes[i] = prometheus.AcceleratorTypeConfig{
				Type:         t.Type,
				Label:        t.Label,
				ResourceName: t.ResourceName,
				MetricName:   t.MetricName,
			}
		}
		acceleratorMetrics, _ = h.promClient.QueryAcceleratorMetrics(ctx, promTypes)
	}

	// 构建响应
	response := GPUOverviewResponse{
		AcceleratorGroups: []AcceleratorGroup{},
		Summary: Summary{
			TotalDevices: 0,
			UsedDevices:  0,
			ByType:       make(map[string]TypeSummary),
		},
		UpdatedAt:         time.Now(),
		PrometheusEnabled: h.promClient != nil && h.promClient.IsEnabled(),
	}

	// 为每种加速卡类型构建分组
	for _, accType := range acceleratorTypes {
		group := h.buildAcceleratorGroup(accType, nodes.Items, pods.Items, acceleratorMetrics)
		if len(group.Nodes) > 0 {
			response.AcceleratorGroups = append(response.AcceleratorGroups, group)
			response.Summary.TotalDevices += group.TotalDevices
			response.Summary.UsedDevices += group.UsedDevices
			response.Summary.ByType[accType.Type] = TypeSummary{
				Total: group.TotalDevices,
				Used:  group.UsedDevices,
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// buildAcceleratorGroup 构建加速卡分组
func (h *ClusterHandler) buildAcceleratorGroup(
	accType models.AcceleratorType,
	nodes []corev1.Node,
	pods []corev1.Pod,
	metrics *prometheus.AcceleratorMetrics,
) AcceleratorGroup {
	group := AcceleratorGroup{
		Type:         accType.Type,
		Label:        accType.Label,
		ResourceName: accType.ResourceName,
		Nodes:        []NodeInfo{},
		TotalDevices: 0,
		UsedDevices:  0,
	}

	// 获取对应类型的指标
	var deviceMetrics []prometheus.DeviceMetric
	if metrics != nil {
		switch accType.Type {
		case "nvidia":
			deviceMetrics = metrics.NvidiaGPUs
		case "ascend":
			deviceMetrics = metrics.AscendNPUs
		}
	}

	// 构建节点到指标的映射
	metricsMap := make(map[string]map[string]prometheus.DeviceMetric) // node -> deviceID -> metric
	for _, m := range deviceMetrics {
		if metricsMap[m.Node] == nil {
			metricsMap[m.Node] = make(map[string]prometheus.DeviceMetric)
		}
		metricsMap[m.Node][m.DeviceID] = m
	}

	// 构建 Pod 到节点的映射，以及统计每个节点的 GPU 使用
	nodePods := make(map[string][]corev1.Pod)
	for _, pod := range pods {
		if pod.Spec.NodeName != "" && pod.Status.Phase == corev1.PodRunning {
			nodePods[pod.Spec.NodeName] = append(nodePods[pod.Spec.NodeName], pod)
		}
	}

	// 遍历节点
	for _, node := range nodes {
		// 检查节点是否有该类型的加速卡
		resourceName := corev1.ResourceName(accType.ResourceName)
		allocatable, ok := node.Status.Allocatable[resourceName]
		if !ok || allocatable.Value() == 0 {
			continue
		}

		totalDevices := int(allocatable.Value())

		// 检测时分复用配置
		timeSharingEnabled, timeSharingReplicas := detectTimeSharing(node, resourceName)

		nodeInfo := NodeInfo{
			NodeName:            node.Name,
			NodeIP:              getNodeIP(node),
			DeviceType:          getDeviceType(node, accType.Type),
			TotalDevices:        totalDevices,
			UsedDevices:         0,
			Slots:               make([]DeviceSlot, totalDevices),
			TimeSharingEnabled:  timeSharingEnabled,
			TimeSharingReplicas: timeSharingReplicas,
		}

		// 初始化所有槽位
		for i := 0; i < totalDevices; i++ {
			nodeInfo.Slots[i] = DeviceSlot{
				Index:       i,
				Status:      "free",
				Utilization: 0,
				Pod:         nil,
			}
		}

		// 从 Prometheus 指标填充槽位信息
		if nodeMetrics, ok := metricsMap[node.Name]; ok {
			for deviceID, metric := range nodeMetrics {
				idx := prometheus.ParseDeviceID(deviceID)
				if idx >= 0 && idx < totalDevices {
					nodeInfo.Slots[idx].Utilization = metric.Utilization
					if metric.Pod != "" {
						nodeInfo.Slots[idx].Status = "used"
						nodeInfo.Slots[idx].Pod = &PodInfo{
							Name:      metric.Pod,
							Namespace: metric.Namespace,
							User:      extractUsername(metric.Pod),
						}
						nodeInfo.UsedDevices++
					}
				}
			}
		}

		// 如果没有 Prometheus 数据，从 K8s Pod 信息推断
		if len(metricsMap[node.Name]) == 0 {
			slotIndex := 0
			for _, pod := range nodePods[node.Name] {
				gpuCount := getPodGPUCount(pod, resourceName)
				if gpuCount > 0 {
					for i := 0; i < gpuCount && slotIndex < totalDevices; i++ {
						nodeInfo.Slots[slotIndex].Status = "used"
						nodeInfo.Slots[slotIndex].Pod = &PodInfo{
							Name:      pod.Name,
							Namespace: pod.Namespace,
							User:      extractUsernameFromPod(pod),
							Email:     extractEmailFromPod(pod),
							GPUCount:  gpuCount,
							StartTime: formatStartTime(pod.Status.StartTime),
						}
						slotIndex++
						nodeInfo.UsedDevices++
					}
				}
			}
		}

		group.Nodes = append(group.Nodes, nodeInfo)
		group.TotalDevices += nodeInfo.TotalDevices
		group.UsedDevices += nodeInfo.UsedDevices
	}

	// 按节点名排序
	sort.Slice(group.Nodes, func(i, j int) bool {
		return group.Nodes[i].NodeName < group.Nodes[j].NodeName
	})

	return group
}

// getNodeIP 获取节点 IP
func getNodeIP(node corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			return addr.Address
		}
	}
	return ""
}

// getDeviceType 获取设备型号
func getDeviceType(node corev1.Node, accType string) string {
	// 尝试从节点标签获取设备型号
	if accType == "nvidia" {
		if model, ok := node.Labels["nvidia.com/gpu.product"]; ok {
			return model
		}
		if model, ok := node.Labels["gpu-type"]; ok {
			return model
		}
	} else if accType == "ascend" {
		if model, ok := node.Labels["huawei.com/npu-type"]; ok {
			return model
		}
		if model, ok := node.Labels["npu-type"]; ok {
			return model
		}
		return "Ascend 910B"
	}
	return ""
}

// getPodGPUCount 获取 Pod 的 GPU 数量
func getPodGPUCount(pod corev1.Pod, resourceName corev1.ResourceName) int {
	total := 0
	for _, container := range pod.Spec.Containers {
		if quantity, ok := container.Resources.Requests[resourceName]; ok {
			total += int(quantity.Value())
		} else if quantity, ok := container.Resources.Limits[resourceName]; ok {
			total += int(quantity.Value())
		}
	}
	return total
}

// extractUsername 从 Pod 名称提取用户名
func extractUsername(podName string) string {
	// 假设 Pod 名称格式为 username-xxx 或 genet-username-xxx
	if len(podName) > 6 && podName[:6] == "genet-" {
		podName = podName[6:]
	}
	for i, ch := range podName {
		if ch == '-' {
			return podName[:i]
		}
	}
	return podName
}

// extractUsernameFromPod 从 Pod 标签提取用户名
func extractUsernameFromPod(pod corev1.Pod) string {
	if user, ok := pod.Labels["genet.io/user"]; ok {
		return user
	}
	return extractUsername(pod.Name)
}

// extractEmailFromPod 从 Pod 标签提取邮箱
func extractEmailFromPod(pod corev1.Pod) string {
	if email, ok := pod.Annotations["genet.io/email"]; ok {
		return email
	}
	return ""
}

// formatStartTime 格式化启动时间
func formatStartTime(t *metav1.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// detectTimeSharing 检测节点是否支持时分复用
// 返回: (是否启用时分复用, 每卡可共享数)
func detectTimeSharing(node corev1.Node, resourceName corev1.ResourceName) (bool, int) {
	// 方法1: 检查节点标签 nvidia.com/device-plugin.config
	if config, ok := node.Labels["nvidia.com/device-plugin.config"]; ok {
		if config == "time-slicing" || strings.Contains(config, "sharing") {
			// 默认假设 4 个 replicas，实际值需要从 ConfigMap 读取
			return true, 4
		}
	}

	// 方法2: 比较 Capacity vs Allocatable
	// 如果 Allocatable > Capacity，说明启用了时分复用
	capacity, hasCapacity := node.Status.Capacity[resourceName]
	allocatable, hasAllocatable := node.Status.Allocatable[resourceName]

	if hasCapacity && hasAllocatable {
		capacityVal := capacity.Value()
		allocatableVal := allocatable.Value()
		if allocatableVal > capacityVal && capacityVal > 0 {
			// 计算 replicas = allocatable / capacity
			replicas := int(allocatableVal / capacityVal)
			return true, replicas
		}
	}

	// 默认不启用时分复用
	return false, 1
}

// 确保 resource 包被使用
var _ = resource.MustParse
