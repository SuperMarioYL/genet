package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StatefulSetHandler struct {
	k8sClient  *k8s.Client
	config     *models.Config
	log        *zap.Logger
	podHandler *PodHandler
}

func NewStatefulSetHandler(k8sClient *k8s.Client, config *models.Config) *StatefulSetHandler {
	return &StatefulSetHandler{
		k8sClient:  k8sClient,
		config:     config,
		log:        logger.Named("statefulset"),
		podHandler: NewPodHandler(k8sClient, nil, config),
	}
}

func (h *StatefulSetHandler) CreateStatefulSet(c *gin.Context) {
	var req models.StatefulSetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的请求参数: %v", err)})
		return
	}
	if err := ValidateImageName(req.Image); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ValidateCPU(req.CPU); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ValidateMemory(req.Memory); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ValidateMemory(req.ShmSize); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "共享内存格式无效，应为数字+单位（如 1Gi, 512Mi）"})
		return
	}

	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	if req.Name != "" {
		if err := k8s.ValidatePodCustomName(req.Name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	if len(req.UserMounts) > 0 {
		if !h.config.Storage.AllowUserMounts {
			c.JSON(http.StatusForbidden, gin.H{"error": "管理员未开启用户自定义挂载功能"})
			return
		}
		for _, mount := range req.UserMounts {
			if err := h.podHandler.validateUserMount(mount); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
	}

	if err := h.k8sClient.EnsureNamespace(ctx, namespace); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建命名空间失败: %v", err)})
		return
	}

	workloadName := k8s.GenerateStatefulSetName(userIdentifier, req.Name)
	if _, err := h.k8sClient.GetStatefulSet(ctx, namespace, workloadName); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "同名 StatefulSet 已存在，请使用其他名称"})
		return
	}

	if err := h.checkQuota(ctx, namespace, req.Replicas, req.GPUCount); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	selectedNode, sharedTotalDevices, err := h.preparePlacement(ctx, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.k8sClient.EnsureStatefulSetVolumePVCs(ctx, namespace, userIdentifier, workloadName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建存储失败: %v", err)})
		return
	}

	cpu := req.CPU
	memory := req.Memory
	shmSize := req.ShmSize
	if cpu == "" {
		cpu = h.config.UI.DefaultCPU
		if cpu == "" {
			cpu = "4"
		}
	}
	if memory == "" {
		memory = h.config.UI.DefaultMemory
		if memory == "" {
			memory = "8Gi"
		}
	}
	if shmSize == "" {
		shmSize = h.config.UI.DefaultShmSize
		if shmSize == "" {
			shmSize = "1Gi"
		}
	}

	spec := &k8s.StatefulSetSpec{
		Name:                   workloadName,
		Namespace:              namespace,
		Username:               userIdentifier,
		Email:                  email,
		Image:                  req.Image,
		GPUCount:               req.GPUCount,
		GPUType:                req.GPUType,
		CPU:                    cpu,
		Memory:                 memory,
		ShmSize:                shmSize,
		HTTPProxy:              h.config.Proxy.HTTPProxy,
		HTTPSProxy:             h.config.Proxy.HTTPSProxy,
		NoProxy:                h.config.Proxy.NoProxy,
		NodeName:               selectedNode,
		Replicas:               int32(req.Replicas),
		UserMounts:             req.UserMounts,
		SharedNodeTotalDevices: sharedTotalDevices,
	}

	if _, err := h.k8sClient.CreateStatefulSet(ctx, spec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建 StatefulSet 失败: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "StatefulSet 创建成功",
		"id":      workloadName,
		"name":    workloadName,
	})
}

func (h *StatefulSetHandler) ListStatefulSets(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	items, err := h.k8sClient.ListStatefulSets(ctx, namespace)
	if err != nil {
		c.JSON(http.StatusOK, models.StatefulSetListResponse{Items: []models.StatefulSetResponse{}})
		return
	}

	result := make([]models.StatefulSetResponse, 0, len(items))
	for i := range items {
		result = append(result, h.buildStatefulSetResponse(ctx, &items[i]))
	}
	c.JSON(http.StatusOK, models.StatefulSetListResponse{Items: result})
}

func (h *StatefulSetHandler) GetStatefulSet(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	name := c.Param("id")
	sts, err := h.k8sClient.GetStatefulSet(ctx, namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "StatefulSet 不存在"})
		return
	}
	c.JSON(http.StatusOK, h.buildStatefulSetResponse(ctx, sts))
}

func (h *StatefulSetHandler) DeleteStatefulSet(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	name := c.Param("id")
	if err := h.k8sClient.DeleteStatefulSet(ctx, namespace, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.k8sClient.DeleteStatefulSetScopedPVCs(ctx, namespace, name); err != nil {
		h.log.Warn("Failed to delete some StatefulSet PVCs", zap.String("name", name), zap.Error(err))
	}

	h.log.Info("StatefulSet deleted successfully",
		zap.String("userIdentifier", userIdentifier),
		zap.String("name", name))
	c.JSON(http.StatusOK, gin.H{"message": "StatefulSet 删除成功"})
}

func (h *StatefulSetHandler) buildStatefulSetResponse(ctx context.Context, sts *appsv1.StatefulSet) models.StatefulSetResponse {
	pods, err := h.k8sClient.ListStatefulSetPods(ctx, sts.Namespace, sts.Name)
	if err != nil {
		pods = nil
	}
	podResponses := make([]models.PodResponse, 0, len(pods))
	for i := range pods {
		podResponses = append(podResponses, h.podHandler.buildPodResponse(ctx, &pods[i]))
	}
	sort.Slice(podResponses, func(i, j int) bool {
		return podResponses[i].Name < podResponses[j].Name
	})

	display := podDisplayInfo{
		ContainerName: "workspace",
		Image:         sts.Annotations["genet.io/image"],
		GPUType:       sts.Annotations["genet.io/gpu-type"],
		CPU:           sts.Annotations["genet.io/cpu"],
		Memory:        sts.Annotations["genet.io/memory"],
		CreatedAt:     sts.CreationTimestamp.Time,
	}
	if createdAt := sts.Annotations["genet.io/created-at"]; createdAt != "" {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			display.CreatedAt = t
		}
	}
	if gpuCount, err := strconv.Atoi(sts.Annotations["genet.io/gpu-count"]); err == nil {
		display.GPUCount = gpuCount
	}

	status := "Pending"
	replicas := int32(0)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}
	switch {
	case sts.DeletionTimestamp != nil:
		status = "Terminating"
	case sts.Status.ReadyReplicas == replicas && replicas > 0:
		status = "Running"
	case sts.Status.CurrentReplicas > 0:
		status = "Starting"
	}

	return models.StatefulSetResponse{
		ID:            sts.Name,
		Name:          sts.Name,
		Namespace:     sts.Namespace,
		Status:        status,
		Image:         display.Image,
		GPUType:       display.GPUType,
		GPUCount:      display.GPUCount,
		CPU:           display.CPU,
		Memory:        display.Memory,
		Replicas:      replicas,
		ReadyReplicas: sts.Status.ReadyReplicas,
		CreatedAt:     display.CreatedAt,
		ServiceName:   sts.Spec.ServiceName,
		Pods:          podResponses,
	}
}

func (h *StatefulSetHandler) checkQuota(ctx context.Context, namespace string, replicas, gpuCount int) error {
	allPods, err := h.k8sClient.ListAllPods(ctx, namespace)
	if err != nil {
		return fmt.Errorf("获取当前配额失败: %w", err)
	}

	currentPods := len(allPods)
	currentGPU := 0
	for _, pod := range allPods {
		display := h.podHandler.getPodDisplayInfo(&pod)
		currentGPU += display.GPUCount
	}

	if currentPods+replicas > h.config.PodLimitPerUser {
		return fmt.Errorf("已达到 Pod 数量限制: %d/%d", currentPods+replicas, h.config.PodLimitPerUser)
	}
	if currentGPU+(replicas*gpuCount) > h.config.GpuLimitPerUser {
		return fmt.Errorf("已达到 GPU 数量限制: %d/%d", currentGPU+(replicas*gpuCount), h.config.GpuLimitPerUser)
	}
	return nil
}

func (h *StatefulSetHandler) preparePlacement(ctx context.Context, req *models.StatefulSetRequest) (string, int, error) {
	if req.GPUCount <= 0 {
		return req.NodeName, 0, nil
	}

	resourceName := "nvidia.com/gpu"
	var selector map[string]string
	for _, gpuType := range h.config.GPU.AvailableTypes {
		if gpuType.Name != req.GPUType {
			continue
		}
		resourceName = gpuType.ResourceName
		selector = gpuType.NodeSelector
		break
	}

	if h.config.GPU.SchedulingMode != "sharing" {
		if req.NodeName == "" {
			return "", 0, nil
		}
		totalDevices, err := h.validateNodeCapacity(ctx, req.NodeName, selector, resourceName, req.GPUCount*req.Replicas)
		return req.NodeName, totalDevices, err
	}

	if req.NodeName != "" {
		totalDevices, err := h.validateSharingNodeCapacity(ctx, req.NodeName, selector, resourceName, req.GPUCount*req.Replicas)
		return req.NodeName, totalDevices, err
	}

	nodes, err := h.k8sClient.GetClientset().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", 0, fmt.Errorf("获取节点列表失败: %w", err)
	}

	sort.Slice(nodes.Items, func(i, j int) bool {
		return nodes.Items[i].Name < nodes.Items[j].Name
	})
	for _, node := range nodes.Items {
		if !nodeMatchesSelector(&node, selector) {
			continue
		}
		qty, ok := node.Status.Allocatable[corev1.ResourceName(resourceName)]
		if !ok || qty.Value() <= 0 {
			continue
		}
		totalDevices := int(qty.Value())
		maxShare := h.config.GPU.MaxPodsPerGPU
		if maxShare <= 0 {
			maxShare = 1
		}
		if totalDevices*maxShare >= req.GPUCount*req.Replicas {
			return node.Name, totalDevices, nil
		}
	}

	return "", 0, fmt.Errorf("没有找到可容纳 %d 个副本的共享节点", req.Replicas)
}

func (h *StatefulSetHandler) validateNodeCapacity(ctx context.Context, nodeName string, selector map[string]string, resourceName string, required int) (int, error) {
	node, err := h.k8sClient.GetClientset().CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("指定的节点不存在: %s", nodeName)
	}
	if !nodeMatchesSelector(node, selector) {
		return 0, fmt.Errorf("节点 %s 不匹配当前计算类型", nodeName)
	}
	qty := node.Status.Allocatable[corev1.ResourceName(resourceName)]
	totalDevices := int(qty.Value())
	if required > 0 && totalDevices < required {
		return totalDevices, fmt.Errorf("节点 %s 的 GPU 容量不足", nodeName)
	}
	return totalDevices, nil
}

func (h *StatefulSetHandler) validateSharingNodeCapacity(ctx context.Context, nodeName string, selector map[string]string, resourceName string, required int) (int, error) {
	totalDevices, err := h.validateNodeCapacity(ctx, nodeName, selector, resourceName, 1)
	if err != nil {
		return 0, err
	}
	maxShare := h.config.GPU.MaxPodsPerGPU
	if maxShare <= 0 {
		maxShare = 1
	}
	if totalDevices*maxShare < required {
		return totalDevices, fmt.Errorf("节点 %s 的共享 GPU 容量不足", nodeName)
	}
	return totalDevices, nil
}

func nodeMatchesSelector(node *corev1.Node, selector map[string]string) bool {
	if node == nil || len(selector) == 0 {
		return true
	}
	for key, value := range selector {
		if node.Labels[key] != value {
			return false
		}
	}
	return true
}
