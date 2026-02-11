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
	Status      string   `json:"status"`      // "free" | "used" | "full"（共享模式已满）
	Utilization float64  `json:"utilization"` // 利用率 0-100
	MemoryUsed  float64  `json:"memoryUsed"`  // 已用显存 (MiB)
	MemoryTotal float64  `json:"memoryTotal"` // 总显存 (MiB)
	Pod         *PodInfo `json:"pod"`         // 占用的 Pod 信息（独占模式）
	// 共享模式字段
	SharedPods   []PodInfo `json:"sharedPods,omitempty"` // 共享该卡的所有 Pod
	CurrentShare int       `json:"currentShare"`         // 当前共享数
	MaxShare     int       `json:"maxShare"`             // 共享上限，0 表示不限
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
	SchedulingMode    string             `json:"schedulingMode"`    // 调度模式: "sharing" | "exclusive"
	MaxPodsPerGPU     int                `json:"maxPodsPerGPU"`     // 每卡最大共享数（共享模式）
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
		h.log.Info("Prometheus is enabled, querying accelerator metrics",
			zap.Int("acceleratorTypes", len(acceleratorTypes)))

		promTypes := make([]prometheus.AcceleratorTypeConfig, len(acceleratorTypes))
		for i, t := range acceleratorTypes {
			h.log.Debug("Accelerator type config",
				zap.String("type", t.Type),
				zap.String("metricName", t.MetricName))

			promTypes[i] = prometheus.AcceleratorTypeConfig{
				Type:         t.Type,
				Label:        t.Label,
				ResourceName: t.ResourceName,
				MetricName:   t.MetricName,
				MetricLabels: prometheus.MetricLabelConfig{
					DeviceID:  t.MetricLabels.DeviceID,
					Node:      t.MetricLabels.Node,
					Pod:       t.MetricLabels.Pod,
					Namespace: t.MetricLabels.Namespace,
				},
			}
		}
		var err error
		acceleratorMetrics, err = h.promClient.QueryAcceleratorMetrics(ctx, promTypes)
		if err != nil {
			h.log.Error("Failed to query accelerator metrics", zap.Error(err))
		} else {
			h.log.Debug("Got accelerator metrics",
				zap.Int("nvidiaGPUs", len(acceleratorMetrics.NvidiaGPUs)),
				zap.Int("ascendNPUs", len(acceleratorMetrics.AscendNPUs)))
		}
	} else {
		h.log.Info("Prometheus is not enabled or client is nil",
			zap.Bool("clientNil", h.promClient == nil),
			zap.Bool("enabled", h.promClient != nil && h.promClient.IsEnabled()))
	}

	// 获取调度模式配置
	schedulingMode := h.config.GPU.SchedulingMode
	if schedulingMode == "" {
		schedulingMode = "exclusive" // 默认独占模式
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
		SchedulingMode:    schedulingMode,
		MaxPodsPerGPU:     h.config.GPU.MaxPodsPerGPU,
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
		// 检查节点是否有该类型的加速卡（用 Capacity 判断物理设备数）
		resourceName := corev1.ResourceName(accType.ResourceName)
		capacity, hasCapacity := node.Status.Capacity[resourceName]
		allocatable, hasAllocatable := node.Status.Allocatable[resourceName]
		if !hasCapacity && !hasAllocatable {
			continue
		}
		// 优先用 Capacity（物理设备数），Allocatable 可能因 device plugin 问题偏小
		capacityVal := int64(0)
		if hasCapacity {
			capacityVal = capacity.Value()
		}
		allocatableVal := int64(0)
		if hasAllocatable {
			allocatableVal = allocatable.Value()
		}
		if capacityVal == 0 && allocatableVal == 0 {
			continue
		}
		totalDevices := int(capacityVal)
		if totalDevices == 0 {
			totalDevices = int(allocatableVal)
		}

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

		// 从 Prometheus 指标填充利用率和显存信息
		if nodeMetrics, ok := metricsMap[node.Name]; ok {
			h.log.Info("Found Prometheus metrics for node",
				zap.String("nodeName", node.Name),
				zap.Int("deviceCount", len(nodeMetrics)))

			for deviceID, metric := range nodeMetrics {
				idx := prometheus.ParseDeviceID(deviceID)
				h.log.Info("Processing device metric",
					zap.String("node", node.Name),
					zap.String("deviceID", deviceID),
					zap.Int("parsedIdx", idx),
					zap.Float64("utilization", metric.Utilization),
					zap.Float64("memoryUsed", metric.MemoryUsed),
					zap.Float64("memoryTotal", metric.MemoryTotal))

				if idx >= 0 && idx < totalDevices {
					nodeInfo.Slots[idx].Utilization = metric.Utilization
					nodeInfo.Slots[idx].MemoryUsed = metric.MemoryUsed
					nodeInfo.Slots[idx].MemoryTotal = metric.MemoryTotal
				}
			}
		} else {
			h.log.Info("No Prometheus metrics found for node",
				zap.String("nodeName", node.Name),
				zap.Any("availableNodes", getMapKeys(metricsMap)))
		}

		// 获取调度模式配置
		isSharing := h.config.GPU.SchedulingMode == "sharing"
		maxPodsPerGPU := h.config.GPU.MaxPodsPerGPU

		// 设置每个槽位的共享上限
		for i := range nodeInfo.Slots {
			nodeInfo.Slots[i].MaxShare = maxPodsPerGPU
		}

		// 从多数据源获取 GPU 使用情况
		gpuUsage := h.getGPUUsageForNode(node.Name, accType.Type, nodePods[node.Name], deviceMetrics, resourceName)

		// 填充槽位信息
		usedSlots := make(map[int]bool)
		for deviceIdx, podInfos := range gpuUsage {
			if deviceIdx < 0 || deviceIdx >= totalDevices {
				continue
			}
			slot := &nodeInfo.Slots[deviceIdx]
			slot.SharedPods = podInfos
			slot.CurrentShare = len(podInfos)

			if len(podInfos) > 0 {
				usedSlots[deviceIdx] = true
				// 设置状态
				if isSharing {
					if maxPodsPerGPU > 0 && len(podInfos) >= maxPodsPerGPU {
						slot.Status = "full" // 共享模式已满
					} else {
						slot.Status = "used" // 共享模式已用但未满
					}
				} else {
					slot.Status = "used" // 独占模式
				}
				// 兼容：设置第一个 Pod 为主 Pod 信息
				slot.Pod = &podInfos[0]
			}
		}

		// 统计已用设备数
		nodeInfo.UsedDevices = len(usedSlots)

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

// getMapKeys 获取 map 的所有 key（用于调试日志）
func getMapKeys(m map[string]map[string]prometheus.DeviceMetric) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// 确保 resource 包被使用
var _ = resource.MustParse

// getGPUUsageForNode 从多数据源获取指定节点上每张 GPU 的使用情况
// 数据源优先级：1. Prometheus 指标（exported_pod）2. Pod 环境变量 3. K8s 资源请求
func (h *ClusterHandler) getGPUUsageForNode(
	nodeName string,
	deviceType string,
	nodePods []corev1.Pod,
	deviceMetrics []prometheus.DeviceMetric,
	resourceName corev1.ResourceName,
) map[int][]PodInfo {
	result := make(map[int][]PodInfo)
	seen := make(map[string]bool) // podKey -> bool，防止重复

	// 1. 从 Prometheus 指标获取（最准确，真正在运行的）
	// NVIDIA: exported_pod 标签；昇腾: pod_name 标签
	for _, metric := range deviceMetrics {
		if metric.Node != nodeName || metric.Pod == "" {
			continue
		}
		idx := prometheus.ParseDeviceID(metric.DeviceID)
		if idx < 0 {
			continue
		}

		podKey := metric.Namespace + "/" + metric.Pod
		if seen[podKey] {
			continue
		}
		seen[podKey] = true

		// 尝试从 nodePods 中找到该 Pod 以获取更多信息
		podInfo := PodInfo{
			Name:      metric.Pod,
			Namespace: metric.Namespace,
		}
		for _, pod := range nodePods {
			if pod.Name == metric.Pod && pod.Namespace == metric.Namespace {
				podInfo.User = extractUsernameFromPod(pod)
				podInfo.Email = extractEmailFromPod(pod)
				podInfo.StartTime = formatStartTime(pod.Status.StartTime)
				break
			}
		}

		result[idx] = append(result[idx], podInfo)
	}

	// 2. 从 Pod 环境变量获取（声明使用的）
	for _, pod := range nodePods {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		podKey := pod.Namespace + "/" + pod.Name
		if seen[podKey] {
			continue
		}

		// 根据设备类型检查对应的环境变量
		devices := h.getDeviceEnvValue(pod, deviceType)
		if devices == "" || devices == "all" || devices == "none" {
			continue
		}

		seen[podKey] = true
		podInfo := PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			User:      extractUsernameFromPod(pod),
			Email:     extractEmailFromPod(pod),
			StartTime: formatStartTime(pod.Status.StartTime),
		}

		for _, idx := range parseDeviceIndices(devices) {
			result[idx] = append(result[idx], podInfo)
		}
	}

	// 3. 从 K8s 资源请求获取（后备，独占模式，不知道具体卡号）
	// 仅当上述方式都没检测到时使用
	if len(result) == 0 {
		slotIndex := 0
		for _, pod := range nodePods {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}

			podKey := pod.Namespace + "/" + pod.Name
			if seen[podKey] {
				continue
			}

			gpuCount := getPodGPUCount(pod, resourceName)
			if gpuCount > 0 {
				seen[podKey] = true
				podInfo := PodInfo{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					User:      extractUsernameFromPod(pod),
					Email:     extractEmailFromPod(pod),
					GPUCount:  gpuCount,
					StartTime: formatStartTime(pod.Status.StartTime),
				}

				// 由于不知道具体卡号，按顺序分配
				for i := 0; i < gpuCount; i++ {
					result[slotIndex] = append(result[slotIndex], podInfo)
					slotIndex++
				}
			}
		}
	}

	return result
}

// getDeviceEnvValue 从 Pod 中获取设备环境变量的值
func (h *ClusterHandler) getDeviceEnvValue(pod corev1.Pod, deviceType string) string {
	var envName string
	switch deviceType {
	case "ascend":
		envName = "ASCEND_RT_VISIBLE_DEVICES"
	default: // nvidia
		envName = "NVIDIA_VISIBLE_DEVICES"
	}

	for _, container := range pod.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == envName {
				return env.Value
			}
		}
	}
	return ""
}

// parseDeviceIndices 解析设备索引列表字符串（如 "0,1,2"）
func parseDeviceIndices(devices string) []int {
	if devices == "" {
		return nil
	}
	var result []int
	for _, s := range strings.Split(devices, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if idx := prometheus.ParseDeviceID(s); idx >= 0 {
			result = append(result, idx)
		}
	}
	return result
}
