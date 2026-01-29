package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodHandler Pod 处理器
type PodHandler struct {
	k8sClient *k8s.Client
	config    *models.Config
	log       *zap.Logger
}

// NewPodHandler 创建 Pod 处理器
func NewPodHandler(k8sClient *k8s.Client, config *models.Config) *PodHandler {
	return &PodHandler{
		k8sClient: k8sClient,
		config:    config,
		log:       logger.Named("pod"),
	}
}

// ListPods 列出用户的所有 Pod
func (h *PodHandler) ListPods(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)

	h.log.Debug("Listing pods",
		zap.String("user", username),
		zap.String("userIdentifier", userIdentifier),
		zap.String("namespace", namespace))

	ctx := context.Background()

	// 列出用户的 Pod
	pods, err := h.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		h.log.Debug("No pods found or namespace not exists",
			zap.String("user", username),
			zap.String("namespace", namespace),
			zap.Error(err))
		// 如果命名空间不存在，返回空列表
		c.JSON(http.StatusOK, models.PodListResponse{
			Pods: []models.PodResponse{},
			Quota: models.QuotaInfo{
				PodUsed:  0,
				PodLimit: h.config.PodLimitPerUser,
				GpuUsed:  0,
				GpuLimit: h.config.GpuLimitPerUser,
			},
		})
		return
	}

	// 构建响应
	podResponses := []models.PodResponse{}
	totalGPU := 0

	for _, pod := range pods {
		// 解析 GPU 信息
		gpuCount := 0
		if len(pod.Spec.Containers) > 0 {
			gpuQuantity := pod.Spec.Containers[0].Resources.Requests["nvidia.com/gpu"]
			gpuCount = int(gpuQuantity.Value())
		}
		totalGPU += gpuCount

		// 解析时间
		createdAt, _ := time.Parse(time.RFC3339, pod.Annotations["genet.io/created-at"])

		// 解析保护截止时间
		var protectedUntil *time.Time
		if protectedStr := pod.Annotations["genet.io/protected-until"]; protectedStr != "" {
			if t, err := time.Parse(time.RFC3339, protectedStr); err == nil {
				protectedUntil = &t
			}
		}

		// 获取节点 IP
		nodeIP := h.getNodeIP(ctx, pod.Spec.NodeName)

		// 获取容器名称（默认为 workspace）
		containerName := "workspace"
		if len(pod.Spec.Containers) > 0 {
			containerName = pod.Spec.Containers[0].Name
		}

		podResponses = append(podResponses, models.PodResponse{
			ID:             pod.Name,
			Name:           pod.Name,
			Namespace:      namespace,
			Container:      containerName,
			Status:         h.getPodStatus(&pod),
			Phase:          string(pod.Status.Phase),
			Image:          pod.Annotations["genet.io/image"],
			GPUType:        pod.Annotations["genet.io/gpu-type"],
			GPUCount:       gpuCount,
			CPU:            pod.Annotations["genet.io/cpu"],
			Memory:         pod.Annotations["genet.io/memory"],
			CreatedAt:      createdAt,
			NodeIP:         nodeIP,
			ProtectedUntil: protectedUntil,
		})
	}

	response := models.PodListResponse{
		Pods: podResponses,
		Quota: models.QuotaInfo{
			PodUsed:  len(podResponses),
			PodLimit: h.config.PodLimitPerUser,
			GpuUsed:  totalGPU,
			GpuLimit: h.config.GpuLimitPerUser,
		},
	}

	h.log.Info("Pods listed",
		zap.String("user", username),
		zap.Int("count", len(podResponses)),
		zap.Int("totalGPU", totalGPU))

	c.JSON(http.StatusOK, response)
}

// CreatePod 创建 Pod
func (h *PodHandler) CreatePod(c *gin.Context) {
	var req models.PodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Warn("Invalid pod creation request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的请求参数: %v", err)})
		return
	}

	// 输入验证
	if err := ValidateImageName(req.Image); err != nil {
		h.log.Warn("Invalid image name", zap.String("image", req.Image), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ValidateCPU(req.CPU); err != nil {
		h.log.Warn("Invalid CPU value", zap.String("cpu", req.CPU), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ValidateMemory(req.Memory); err != nil {
		h.log.Warn("Invalid memory value", zap.String("memory", req.Memory), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)

	// 使用 username 和邮箱前缀生成用户标识
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	h.log.Info("Creating pod",
		zap.String("user", username),
		zap.String("email", email),
		zap.String("userIdentifier", userIdentifier),
		zap.String("namespace", namespace),
		zap.String("image", req.Image),
		zap.Int("gpuCount", req.GPUCount),
		zap.String("gpuType", req.GPUType),
		zap.String("nodeName", req.NodeName),
		zap.Ints("gpuDevices", req.GPUDevices),
		zap.String("customName", req.Name))

	// 如果指定了 GPUDevices，自动设置 GPUCount
	if len(req.GPUDevices) > 0 {
		// 指定 GPU 卡时必须同时指定节点
		if req.NodeName == "" {
			h.log.Warn("gpuDevices specified without nodeName",
				zap.String("user", username),
				zap.Ints("gpuDevices", req.GPUDevices))
			c.JSON(http.StatusBadRequest, gin.H{"error": "指定 GPU 卡时必须同时指定节点"})
			return
		}
		req.GPUCount = len(req.GPUDevices)
		h.log.Debug("GPU count auto-set from gpuDevices",
			zap.Int("gpuCount", req.GPUCount),
			zap.Ints("gpuDevices", req.GPUDevices))
	}

	// 检查配额
	if err := h.checkQuota(ctx, userIdentifier, namespace, req.GPUCount); err != nil {
		h.log.Warn("Quota exceeded",
			zap.String("user", username),
			zap.String("userIdentifier", userIdentifier),
			zap.Error(err))
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	// 验证 GPU 类型（仅当 GPU 数量 > 0 时）
	if req.GPUCount > 0 && req.GPUType != "" {
		valid := false
		for _, gpuType := range h.config.GPU.AvailableTypes {
			if gpuType.Name == req.GPUType {
				valid = true
				break
			}
		}
		if !valid {
			h.log.Warn("Invalid GPU type",
				zap.String("user", username),
				zap.String("gpuType", req.GPUType))
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 GPU 类型"})
			return
		}
	}

	// 验证指定的节点（如果有）
	if req.NodeName != "" {
		clientset := h.k8sClient.GetClientset()
		node, err := clientset.CoreV1().Nodes().Get(ctx, req.NodeName, metav1.GetOptions{})
		if err != nil {
			h.log.Warn("Specified node not found",
				zap.String("user", username),
				zap.String("nodeName", req.NodeName),
				zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("指定的节点不存在: %s", req.NodeName)})
			return
		}

		// 验证节点 GPU 容量是否足够（如果需要 GPU）
		if req.GPUCount > 0 {
			// 查找对应的资源名称
			resourceName := "nvidia.com/gpu"
			for _, gpuType := range h.config.GPU.AvailableTypes {
				if gpuType.Name == req.GPUType {
					resourceName = gpuType.ResourceName
					break
				}
			}

			allocatable, ok := node.Status.Allocatable[corev1.ResourceName(resourceName)]
			if !ok || allocatable.Value() < int64(req.GPUCount) {
				h.log.Warn("Node GPU capacity insufficient",
					zap.String("user", username),
					zap.String("nodeName", req.NodeName),
					zap.Int64("allocatable", allocatable.Value()),
					zap.Int("requested", req.GPUCount))
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("节点 %s 的 GPU 容量不足", req.NodeName)})
				return
			}

			// 验证指定的 GPU 设备索引是否有效
			if len(req.GPUDevices) > 0 {
				totalDevices := int(allocatable.Value())
				for _, deviceIndex := range req.GPUDevices {
					if deviceIndex < 0 || deviceIndex >= totalDevices {
						h.log.Warn("Invalid GPU device index",
							zap.String("user", username),
							zap.String("nodeName", req.NodeName),
							zap.Int("deviceIndex", deviceIndex),
							zap.Int("totalDevices", totalDevices))
						c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的 GPU 卡编号 %d，节点 %s 共有 %d 张卡", deviceIndex, req.NodeName, totalDevices)})
						return
					}
				}
			}
		}

		h.log.Debug("Node validation passed",
			zap.String("nodeName", req.NodeName))
	}

	// 确保命名空间存在
	h.log.Debug("Ensuring namespace exists", zap.String("namespace", namespace))
	if err := h.k8sClient.EnsureNamespace(ctx, namespace); err != nil {
		h.log.Error("Failed to create namespace",
			zap.String("namespace", namespace),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建命名空间失败: %v", err)})
		return
	}

	// 确保 PVC 存在（用户级别共享）
	storageClass := h.config.Storage.StorageClass
	storageSize := h.config.Storage.Size
	if storageSize == "" {
		storageSize = "50Gi"
	}
	h.log.Debug("Ensuring PVC exists",
		zap.String("namespace", namespace),
		zap.String("storageClass", storageClass),
		zap.String("size", storageSize))
	if err := h.k8sClient.EnsurePVC(ctx, namespace, userIdentifier, storageClass, storageSize); err != nil {
		h.log.Error("Failed to create PVC",
			zap.String("namespace", namespace),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建存储失败: %v", err)})
		return
	}

	// 验证自定义 Pod 名称
	if req.Name != "" {
		if err := k8s.ValidatePodCustomName(req.Name); err != nil {
			h.log.Warn("Invalid custom pod name",
				zap.String("user", username),
				zap.String("customName", req.Name),
				zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// 生成 Pod 名称（支持自定义）
	podName := k8s.GeneratePodName(userIdentifier, req.Name)

	// 检查同名 Pod 是否已存在（仅自定义名称时检查）
	if req.Name != "" {
		if h.k8sClient.PodExists(ctx, namespace, podName) {
			h.log.Warn("Pod with same name already exists",
				zap.String("user", username),
				zap.String("podName", podName))
			c.JSON(http.StatusConflict, gin.H{"error": "同名 Pod 已存在，请使用其他名称"})
			return
		}
	}

	// 使用默认值（如果用户未指定）
	cpu := req.CPU
	memory := req.Memory
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

	// 创建 Pod
	spec := &k8s.PodSpec{
		Name:       podName,
		Namespace:  namespace,
		Username:   userIdentifier, // 使用 userIdentifier 作为存储卷路径中的用户标识
		Email:      email,          // 用户邮箱
		Image:      req.Image,
		GPUCount:   req.GPUCount,
		GPUType:    req.GPUType,
		CPU:        cpu,
		Memory:     memory,
		HTTPProxy:  h.config.Proxy.HTTPProxy,
		HTTPSProxy: h.config.Proxy.HTTPSProxy,
		NoProxy:    h.config.Proxy.NoProxy,
		// 高级配置
		NodeName:   req.NodeName,
		GPUDevices: req.GPUDevices,
	}

	h.log.Debug("Creating pod resource",
		zap.String("podName", podName))

	_, err := h.k8sClient.CreatePod(ctx, spec)
	if err != nil {
		h.log.Error("Failed to create pod",
			zap.String("user", username),
			zap.String("podName", podName),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建 Pod 失败: %v", err)})
		return
	}

	h.log.Info("Pod created successfully",
		zap.String("user", username),
		zap.String("podName", podName),
		zap.String("image", req.Image),
		zap.Int("gpuCount", req.GPUCount))

	c.JSON(http.StatusCreated, gin.H{
		"message": "Pod 创建成功",
		"id":      podName,
		"name":    podName,
	})
}

// GetPod 获取 Pod 详情
func (h *PodHandler) GetPod(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	h.log.Debug("Getting pod details",
		zap.String("user", username),
		zap.String("podID", podID))

	// 获取 Pod
	pod, err := h.k8sClient.GetPod(ctx, namespace, podID)
	if err != nil {
		h.log.Warn("Pod not found",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Pod 不存在"})
		return
	}

	// 解析 GPU 信息
	gpuCount := 0
	if len(pod.Spec.Containers) > 0 {
		gpuQuantity := pod.Spec.Containers[0].Resources.Requests["nvidia.com/gpu"]
		gpuCount = int(gpuQuantity.Value())
	}

	// 解析时间
	createdAt, _ := time.Parse(time.RFC3339, pod.Annotations["genet.io/created-at"])

	// 解析保护截止时间
	var protectedUntil *time.Time
	if protectedStr := pod.Annotations["genet.io/protected-until"]; protectedStr != "" {
		if t, err := time.Parse(time.RFC3339, protectedStr); err == nil {
			protectedUntil = &t
		}
	}

	// 获取节点 IP
	nodeIP := h.getNodeIP(ctx, pod.Spec.NodeName)

	// 获取容器名称（默认为 workspace）
	containerName := "workspace"
	if len(pod.Spec.Containers) > 0 {
		containerName = pod.Spec.Containers[0].Name
	}

	response := models.PodResponse{
		ID:             pod.Name,
		Name:           pod.Name,
		Namespace:      namespace,
		Container:      containerName,
		Status:         h.getPodStatus(pod),
		Phase:          string(pod.Status.Phase),
		Image:          pod.Annotations["genet.io/image"],
		GPUType:        pod.Annotations["genet.io/gpu-type"],
		GPUCount:       gpuCount,
		CPU:            pod.Annotations["genet.io/cpu"],
		Memory:         pod.Annotations["genet.io/memory"],
		CreatedAt:      createdAt,
		NodeIP:         nodeIP,
		ProtectedUntil: protectedUntil,
	}

	c.JSON(http.StatusOK, response)
}

// ExtendPod 延长 Pod 保护期
// 设置保护截止时间为明天 22:59，跳过下一次 23:00 清理
func (h *PodHandler) ExtendPod(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	h.log.Info("Extending pod protection",
		zap.String("user", username),
		zap.String("podID", podID),
		zap.String("namespace", namespace))

	// 获取 Pod
	pod, err := h.k8sClient.GetPod(ctx, namespace, podID)
	if err != nil {
		h.log.Warn("Pod not found for extension",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Pod 不存在"})
		return
	}

	// 计算保护截止时间：明天 22:59
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)
	protectedUntil := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 22, 59, 0, 0, now.Location())

	// 更新 Pod 注解
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations["genet.io/protected-until"] = protectedUntil.Format(time.RFC3339)

	// 更新 Pod
	clientset := h.k8sClient.GetClientset()
	_, err = clientset.CoreV1().Pods(namespace).Update(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		h.log.Error("Failed to extend pod protection",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("延长保护失败: %v", err)})
		return
	}

	h.log.Info("Pod protection extended",
		zap.String("user", username),
		zap.String("podID", podID),
		zap.Time("protectedUntil", protectedUntil))

	c.JSON(http.StatusOK, gin.H{
		"message":        "Pod 保护已延长",
		"protectedUntil": protectedUntil,
	})
}

// DeletePod 删除 Pod
func (h *PodHandler) DeletePod(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	h.log.Info("Deleting pod",
		zap.String("user", username),
		zap.String("podID", podID),
		zap.String("namespace", namespace))

	// 检查是否有正在运行的 commit job
	commitStatus, err := h.k8sClient.GetCommitJobStatus(ctx, namespace, podID)
	if err == nil && commitStatus != nil {
		if commitStatus.Status == "Running" || commitStatus.Status == "Pending" {
			h.log.Warn("Cannot delete pod with active commit job",
				zap.String("user", username),
				zap.String("podID", podID),
				zap.String("jobStatus", commitStatus.Status))
			c.JSON(http.StatusConflict, gin.H{
				"error": fmt.Sprintf("Pod 有正在进行的镜像保存任务（%s），请等待完成后再删除", commitStatus.Status),
			})
			return
		}
	}

	// 删除 Pod
	err = h.k8sClient.DeletePod(ctx, namespace, podID)
	if err != nil {
		h.log.Error("Failed to delete pod",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("删除 Pod 失败: %v", err)})
		return
	}

	h.log.Info("Pod deleted successfully",
		zap.String("user", username),
		zap.String("podID", podID))

	// 根据 ReclaimPolicy 决定是否删除 PVC
	storageVolumes := h.k8sClient.GetStorageVolumes()
	for _, vol := range storageVolumes {
		// 只处理 PVC 类型且 ReclaimPolicy 为 Delete 的存储卷
		if vol.Type == "pvc" && strings.ToLower(vol.ReclaimPolicy) == "delete" {
			pvcName := h.k8sClient.GetPVCName(vol, username)
			if pvcName != "" {
				h.log.Info("Deleting PVC due to ReclaimPolicy=Delete",
					zap.String("pvcName", pvcName),
					zap.String("volumeName", vol.Name),
					zap.String("user", username))

				if err := h.k8sClient.DeletePVC(ctx, namespace, pvcName); err != nil {
					h.log.Warn("Failed to delete PVC (continuing anyway)",
						zap.String("pvcName", pvcName),
						zap.Error(err))
				} else {
					h.log.Info("PVC deleted successfully",
						zap.String("pvcName", pvcName))
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Pod 删除成功"})
}

// GetPodLogs 获取 Pod 日志
func (h *PodHandler) GetPodLogs(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	h.log.Debug("Getting pod logs",
		zap.String("user", username),
		zap.String("podID", podID))

	// 获取日志
	clientset := h.k8sClient.GetClientset()
	tailLines := int64(100)
	req := clientset.CoreV1().Pods(namespace).GetLogs(podID, &corev1.PodLogOptions{
		TailLines: &tailLines,
	})

	logs, err := req.Stream(ctx)
	if err != nil {
		h.log.Error("Failed to get pod logs",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取日志失败: %v", err)})
		return
	}
	defer logs.Close()

	// 读取日志
	buf := new(strings.Builder)
	_, err = io.Copy(buf, logs)
	if err != nil {
		h.log.Error("Failed to read pod logs",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("读取日志失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": buf.String()})
}

// CommitImageRequest 镜像 commit 请求
type CommitImageRequest struct {
	ImageName string `json:"imageName" binding:"required"` // 目标镜像名称（包含 tag）
}

// CommitImage 将 Pod 保存为镜像（类似 docker commit）
func (h *PodHandler) CommitImage(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := c.Request.Context()

	// 解析请求
	var req CommitImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Warn("Invalid commit request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "请指定目标镜像名称"})
		return
	}

	h.log.Info("Committing pod to image",
		zap.String("user", username),
		zap.String("podID", podID),
		zap.String("targetImage", req.ImageName))

	// 获取 Pod 信息
	pod, err := h.k8sClient.GetPod(ctx, namespace, podID)
	if err != nil {
		h.log.Warn("Pod not found for commit",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Pod 不存在"})
		return
	}

	// 检查 Pod 是否在运行
	if pod.Status.Phase != "Running" {
		h.log.Warn("Pod not running, cannot commit",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.String("phase", string(pod.Status.Phase)))
		c.JSON(http.StatusBadRequest, gin.H{"error": "只能对运行中的 Pod 进行镜像保存"})
		return
	}

	// 构建 commit spec
	spec := &k8s.CommitSpec{
		PodName:     podID,
		Namespace:   namespace,
		Username:    username,
		TargetImage: req.ImageName,
		NodeName:    pod.Spec.NodeName,
	}

	// 创建 commit job
	job, err := h.k8sClient.CreateCommitJob(ctx, spec)
	if err != nil {
		h.log.Error("Failed to create commit job",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建 commit 任务失败: %v", err)})
		return
	}

	h.log.Info("Commit job created",
		zap.String("user", username),
		zap.String("podID", podID),
		zap.String("jobName", job.Name),
		zap.String("targetImage", req.ImageName))

	c.JSON(http.StatusOK, gin.H{
		"message":     "镜像保存任务已创建",
		"jobName":     job.Name,
		"targetImage": req.ImageName,
	})
}

// GetCommitStatus 获取 commit 任务状态
func (h *PodHandler) GetCommitStatus(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := c.Request.Context()

	h.log.Debug("Getting commit status",
		zap.String("user", username),
		zap.String("podID", podID))

	status, err := h.k8sClient.GetCommitJobStatus(ctx, namespace, podID)
	if err != nil {
		h.log.Error("Failed to get commit status",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取状态失败: %v", err)})
		return
	}

	if status == nil {
		c.JSON(http.StatusOK, gin.H{
			"hasJob":  false,
			"message": "没有进行中的镜像保存任务",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"hasJob":    true,
		"jobName":   status.JobName,
		"status":    status.Status,
		"message":   status.Message,
		"startTime": status.StartTime,
		"endTime":   status.EndTime,
	})
}

// GetCommitLogs 获取 commit 任务日志
func (h *PodHandler) GetCommitLogs(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := c.Request.Context()

	h.log.Debug("Getting commit logs",
		zap.String("user", username),
		zap.String("podID", podID))

	logs, err := h.k8sClient.GetCommitJobLogs(ctx, namespace, podID)
	if err != nil {
		h.log.Error("Failed to get commit logs",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取日志失败: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// BuildImage 触发镜像构建（保留原接口，重定向到 CommitImage）
func (h *PodHandler) BuildImage(c *gin.Context) {
	h.CommitImage(c)
}

// GetPodEvents 获取 Pod 事件（类似 kubectl describe 的 Events 部分）
func (h *PodHandler) GetPodEvents(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)

	h.log.Debug("Getting pod events",
		zap.String("user", username),
		zap.String("podID", podID))

	ctx := c.Request.Context()
	clientset := h.k8sClient.GetClientset()

	// 获取 Pod 相关的 Events
	events, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Pod", podID),
	})
	if err != nil {
		h.log.Error("Failed to get pod events",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("获取事件失败: %v", err)})
		return
	}

	// 格式化事件
	var eventList []map[string]interface{}
	for _, event := range events.Items {
		eventList = append(eventList, map[string]interface{}{
			"type":      event.Type,
			"reason":    event.Reason,
			"message":   event.Message,
			"count":     event.Count,
			"firstTime": event.FirstTimestamp.Time,
			"lastTime":  event.LastTimestamp.Time,
			"source":    event.Source.Component,
		})
	}

	c.JSON(http.StatusOK, gin.H{"events": eventList})
}

// GetPodDescribe 获取 Pod 详细描述信息
func (h *PodHandler) GetPodDescribe(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)

	h.log.Debug("Getting pod description",
		zap.String("user", username),
		zap.String("podID", podID))

	ctx := c.Request.Context()
	clientset := h.k8sClient.GetClientset()

	// 获取 Pod
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podID, metav1.GetOptions{})
	if err != nil {
		h.log.Warn("Pod not found for describe",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Pod 不存在: %v", err)})
		return
	}

	// 构建描述信息
	describe := map[string]interface{}{
		"name":        pod.Name,
		"namespace":   pod.Namespace,
		"node":        pod.Spec.NodeName,
		"status":      string(pod.Status.Phase),
		"ip":          pod.Status.PodIP,
		"hostIP":      pod.Status.HostIP,
		"startTime":   pod.Status.StartTime,
		"labels":      pod.Labels,
		"annotations": pod.Annotations,
	}

	// 容器状态
	var containerStatuses []map[string]interface{}
	for _, cs := range pod.Status.ContainerStatuses {
		status := map[string]interface{}{
			"name":         cs.Name,
			"ready":        cs.Ready,
			"restartCount": cs.RestartCount,
			"image":        cs.Image,
		}
		if cs.State.Running != nil {
			status["state"] = "Running"
			status["startedAt"] = cs.State.Running.StartedAt.Time
		} else if cs.State.Waiting != nil {
			status["state"] = "Waiting"
			status["reason"] = cs.State.Waiting.Reason
			status["message"] = cs.State.Waiting.Message
		} else if cs.State.Terminated != nil {
			status["state"] = "Terminated"
			status["reason"] = cs.State.Terminated.Reason
			status["exitCode"] = cs.State.Terminated.ExitCode
			status["message"] = cs.State.Terminated.Message
		}
		containerStatuses = append(containerStatuses, status)
	}
	describe["containers"] = containerStatuses

	// 条件
	var conditions []map[string]interface{}
	for _, cond := range pod.Status.Conditions {
		conditions = append(conditions, map[string]interface{}{
			"type":    string(cond.Type),
			"status":  string(cond.Status),
			"reason":  cond.Reason,
			"message": cond.Message,
		})
	}
	describe["conditions"] = conditions

	c.JSON(http.StatusOK, describe)
}

// checkQuota 检查用户配额
func (h *PodHandler) checkQuota(ctx context.Context, userIdentifier, namespace string, requestGPUCount int) error {
	// 列出用户的 Pod
	pods, err := h.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		// 如果命名空间不存在，视为没有 Pod
		h.log.Debug("Namespace not exists, quota check passed",
			zap.String("userIdentifier", userIdentifier),
			zap.String("namespace", namespace))
		return nil
	}

	// 检查 Pod 数量限制
	if len(pods) >= h.config.PodLimitPerUser {
		return fmt.Errorf("已达到 Pod 数量限制: %d/%d", len(pods), h.config.PodLimitPerUser)
	}

	// 检查 GPU 总数限制
	totalGPU := 0
	for _, pod := range pods {
		if len(pod.Spec.Containers) > 0 {
			gpuQuantity := pod.Spec.Containers[0].Resources.Requests["nvidia.com/gpu"]
			totalGPU += int(gpuQuantity.Value())
		}
	}

	if totalGPU+requestGPUCount > h.config.GpuLimitPerUser {
		return fmt.Errorf("GPU 总数超限: 当前 %d，请求 %d，限制 %d",
			totalGPU, requestGPUCount, h.config.GpuLimitPerUser)
	}

	h.log.Debug("Quota check passed",
		zap.String("userIdentifier", userIdentifier),
		zap.Int("currentPods", len(pods)),
		zap.Int("podLimit", h.config.PodLimitPerUser),
		zap.Int("currentGPU", totalGPU),
		zap.Int("requestGPU", requestGPUCount),
		zap.Int("gpuLimit", h.config.GpuLimitPerUser))

	return nil
}

// getNodeIP 获取节点 IP
func (h *PodHandler) getNodeIP(ctx context.Context, nodeName string) string {
	if nodeName == "" {
		return ""
	}

	clientset := h.k8sClient.GetClientset()
	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		h.log.Debug("Failed to get node IP",
			zap.String("nodeName", nodeName),
			zap.Error(err))
		return ""
	}

	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP || addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}

	return ""
}

// SharedGPUPod 共用 GPU 的 Pod 信息
type SharedGPUPod struct {
	Name       string `json:"name"`       // Pod 名称
	Namespace  string `json:"namespace"`  // 命名空间
	User       string `json:"user"`       // 用户名
	GPUDevices []int  `json:"gpuDevices"` // 该 Pod 使用的所有 GPU
	SharedWith []int  `json:"sharedWith"` // 与当前 Pod 共用的 GPU 编号
	CreatedAt  string `json:"createdAt"`  // 创建时间
}

// SharedGPUPodsResponse 共用 GPU 的 Pod 响应
type SharedGPUPodsResponse struct {
	Pods []SharedGPUPod `json:"pods"`
}

// GetSharedGPUPods 获取与当前 Pod 共用 GPU 的其他 Pod
func (h *PodHandler) GetSharedGPUPods(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := context.Background()

	h.log.Debug("Getting shared GPU pods",
		zap.String("user", username),
		zap.String("podID", podID))

	// 获取当前 Pod
	pod, err := h.k8sClient.GetPod(ctx, namespace, podID)
	if err != nil {
		h.log.Warn("Pod not found",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "Pod 不存在"})
		return
	}

	// 如果 Pod 没有调度到节点，返回空列表
	if pod.Spec.NodeName == "" {
		c.JSON(http.StatusOK, SharedGPUPodsResponse{Pods: []SharedGPUPod{}})
		return
	}

	// 获取当前 Pod 的 GPU 设备列表
	currentGPUDevices := parseGPUDevicesFromAnnotation(pod.Annotations["genet.io/gpu-devices"])
	if len(currentGPUDevices) == 0 {
		// 如果没有指定具体设备，返回空列表
		c.JSON(http.StatusOK, SharedGPUPodsResponse{Pods: []SharedGPUPod{}})
		return
	}

	// 获取同节点的所有 Pod
	clientset := h.k8sClient.GetClientset()
	allPods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", pod.Spec.NodeName),
	})
	if err != nil {
		h.log.Error("Failed to list pods on node",
			zap.String("nodeName", pod.Spec.NodeName),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取节点 Pod 列表失败"})
		return
	}

	// 找出共用 GPU 的 Pod
	var sharedPods []SharedGPUPod
	currentGPUSet := make(map[int]bool)
	for _, d := range currentGPUDevices {
		currentGPUSet[d] = true
	}

	for _, otherPod := range allPods.Items {
		// 跳过当前 Pod 自己
		if otherPod.Name == podID && otherPod.Namespace == namespace {
			continue
		}

		// 跳过非运行状态的 Pod
		if otherPod.Status.Phase != corev1.PodRunning {
			continue
		}

		// 获取其他 Pod 的 GPU 设备列表
		otherGPUDevices := parseGPUDevicesFromAnnotation(otherPod.Annotations["genet.io/gpu-devices"])
		if len(otherGPUDevices) == 0 {
			continue
		}

		// 找出共用的 GPU
		var sharedWith []int
		for _, d := range otherGPUDevices {
			if currentGPUSet[d] {
				sharedWith = append(sharedWith, d)
			}
		}

		if len(sharedWith) > 0 {
			// 获取用户名
			user := otherPod.Labels["genet.io/user"]
			if user == "" {
				user = "unknown"
			}

			// 获取创建时间
			createdAt := ""
			if otherPod.Annotations["genet.io/created-at"] != "" {
				createdAt = otherPod.Annotations["genet.io/created-at"]
			} else if otherPod.CreationTimestamp.Time.Year() > 1 {
				createdAt = otherPod.CreationTimestamp.Format(time.RFC3339)
			}

			sharedPods = append(sharedPods, SharedGPUPod{
				Name:       otherPod.Name,
				Namespace:  otherPod.Namespace,
				User:       user,
				GPUDevices: otherGPUDevices,
				SharedWith: sharedWith,
				CreatedAt:  createdAt,
			})
		}
	}

	c.JSON(http.StatusOK, SharedGPUPodsResponse{Pods: sharedPods})
}

// parseGPUDevicesFromAnnotation 从 annotation 解析 GPU 设备列表
// 格式: "0,2,5" -> [0, 2, 5]
func parseGPUDevicesFromAnnotation(annotation string) []int {
	if annotation == "" {
		return nil
	}

	parts := strings.Split(annotation, ",")
	var devices []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var d int
		if _, err := fmt.Sscanf(p, "%d", &d); err == nil {
			devices = append(devices, d)
		}
	}
	return devices
}

// getPodStatus 获取 Pod 状态（模拟 kubectl get pod 的 STATUS 列）
func (h *PodHandler) getPodStatus(pod *corev1.Pod) string {
	// 如果正在删除
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}

	// 检查容器状态
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			reason := cs.State.Waiting.Reason
			if reason != "" {
				return reason // ContainerCreating, CrashLoopBackOff, ImagePullBackOff, etc.
			}
		}
		if cs.State.Terminated != nil {
			reason := cs.State.Terminated.Reason
			if reason != "" {
				return reason // Error, Completed, OOMKilled, etc.
			}
		}
	}

	// 检查 Init 容器状态
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.State.Waiting != nil {
			reason := cs.State.Waiting.Reason
			if reason != "" {
				return "Init:" + reason
			}
		}
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			return "Init:Error"
		}
	}

	// 默认返回 Phase
	return string(pod.Status.Phase)
}
