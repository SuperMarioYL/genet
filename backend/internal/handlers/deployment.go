package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

type DeploymentHandler struct {
	k8sClient  *k8s.Client
	config     *models.Config
	log        *zap.Logger
	podHandler *PodHandler
}

func NewDeploymentHandler(k8sClient *k8s.Client, config *models.Config) *DeploymentHandler {
	return &DeploymentHandler{
		k8sClient:  k8sClient,
		config:     config,
		log:        logger.Named("deployment"),
		podHandler: NewPodHandler(k8sClient, nil, config),
	}
}

func (h *DeploymentHandler) CreateDeployment(c *gin.Context) {
	var req models.DeploymentRequest
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

	if req.Replicas > 1 && h.k8sClient.HasPodScopedPVCVolumes() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Deployment 多副本暂不支持 scope=pod PVC，请改用 StatefulSet 或调整存储配置"})
		return
	}

	if err := h.k8sClient.EnsureNamespace(ctx, namespace); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建命名空间失败: %v", err)})
		return
	}

	workloadName := k8s.GenerateDeploymentName(userIdentifier, req.Name)
	if _, err := h.k8sClient.GetDeployment(ctx, namespace, workloadName); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "同名 Deployment 已存在，请使用其他名称"})
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

	spec := &k8s.DeploymentSpec{
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

	if _, err := h.k8sClient.CreateDeployment(ctx, spec); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "scope=pod PVC") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": fmt.Sprintf("创建 Deployment 失败: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Deployment 创建成功",
		"id":      workloadName,
		"name":    workloadName,
	})
}

func (h *DeploymentHandler) ListDeployments(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	items, err := h.k8sClient.ListDeployments(ctx, namespace)
	if err != nil {
		c.JSON(http.StatusOK, models.DeploymentListResponse{Items: []models.DeploymentResponse{}})
		return
	}

	result := make([]models.DeploymentResponse, 0, len(items))
	for i := range items {
		result = append(result, h.buildDeploymentResponse(ctx, &items[i]))
	}
	c.JSON(http.StatusOK, models.DeploymentListResponse{Items: result})
}

func (h *DeploymentHandler) GetDeployment(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	name := c.Param("id")
	deploy, err := h.k8sClient.GetDeployment(ctx, namespace, name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deployment 不存在"})
		return
	}
	c.JSON(http.StatusOK, h.buildDeploymentResponse(ctx, deploy))
}

func (h *DeploymentHandler) DeleteDeployment(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	name := c.Param("id")
	if err := h.k8sClient.DeleteDeployment(ctx, namespace, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.log.Info("Deployment deleted successfully",
		zap.String("name", name),
		zap.String("namespace", namespace))
	c.JSON(http.StatusOK, gin.H{"message": "Deployment 删除成功"})
}

func (h *DeploymentHandler) buildDeploymentResponse(ctx context.Context, deploy *appsv1.Deployment) models.DeploymentResponse {
	pods, err := h.k8sClient.ListDeploymentPods(ctx, deploy.Namespace, deploy.Name)
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
		Image:         deploy.Annotations["genet.io/image"],
		GPUType:       deploy.Annotations["genet.io/gpu-type"],
		CPU:           deploy.Annotations["genet.io/cpu"],
		Memory:        deploy.Annotations["genet.io/memory"],
		CreatedAt:     deploy.CreationTimestamp.Time,
	}
	if createdAt := deploy.Annotations["genet.io/created-at"]; createdAt != "" {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			display.CreatedAt = t
		}
	}
	if gpuCount, err := strconv.Atoi(deploy.Annotations["genet.io/gpu-count"]); err == nil {
		display.GPUCount = gpuCount
	}

	status := "Pending"
	replicas := int32(0)
	if deploy.Spec.Replicas != nil {
		replicas = *deploy.Spec.Replicas
	}
	switch {
	case deploy.DeletionTimestamp != nil:
		status = "Terminating"
	case deploy.Status.ReadyReplicas == replicas && replicas > 0:
		status = "Running"
	case deploy.Status.AvailableReplicas > 0 || deploy.Status.UpdatedReplicas > 0 || deploy.Status.Replicas > 0:
		status = "Starting"
	}

	return models.DeploymentResponse{
		ID:            deploy.Name,
		Name:          deploy.Name,
		Namespace:     deploy.Namespace,
		Status:        status,
		Image:         display.Image,
		GPUType:       display.GPUType,
		GPUCount:      display.GPUCount,
		CPU:           display.CPU,
		Memory:        display.Memory,
		Replicas:      replicas,
		ReadyReplicas: deploy.Status.ReadyReplicas,
		CreatedAt:     display.CreatedAt,
		Pods:          podResponses,
	}
}

func (h *DeploymentHandler) checkQuota(ctx context.Context, namespace string, replicas, gpuCount int) error {
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

func (h *DeploymentHandler) preparePlacement(ctx context.Context, req *models.DeploymentRequest) (string, int, error) {
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

func (h *DeploymentHandler) validateNodeCapacity(ctx context.Context, nodeName string, selector map[string]string, resourceName string, required int) (int, error) {
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

func (h *DeploymentHandler) validateSharingNodeCapacity(ctx context.Context, nodeName string, selector map[string]string, resourceName string, required int) (int, error) {
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
