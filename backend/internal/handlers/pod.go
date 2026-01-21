package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	namespace := k8s.GetNamespaceForUser(username)

	h.log.Debug("Listing pods",
		zap.String("user", username),
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
		expiresAt, _ := time.Parse(time.RFC3339, pod.Annotations["genet.io/expires-at"])

		// 获取节点 IP
		nodeIP := h.getNodeIP(ctx, pod.Spec.NodeName)

		// 获取 SSH 端口
		sshPort := int32(0)
		if portStr, ok := pod.Annotations["genet.io/ssh-port"]; ok {
			if port, err := strconv.ParseInt(portStr, 10, 32); err == nil {
				sshPort = int32(port)
			}
		}

		// 获取密码
		password := pod.Annotations["genet.io/password"]

		// 构建连接信息
		connections := h.buildConnectionInfo(nodeIP, sshPort, password, pod.Name)

		podResponses = append(podResponses, models.PodResponse{
			ID:          pod.Name,
			Name:        pod.Name,
			Status:      h.getPodStatus(&pod),
			Phase:       string(pod.Status.Phase),
			Image:       pod.Annotations["genet.io/image"],
			GPUType:     pod.Annotations["genet.io/gpu-type"],
			GPUCount:    gpuCount,
			CreatedAt:   createdAt,
			ExpiresAt:   expiresAt,
			NodeIP:      nodeIP,
			Connections: connections,
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

	username, _ := auth.GetUsername(c)
	namespace := k8s.GetNamespaceForUser(username)
	ctx := context.Background()

	h.log.Info("Creating pod",
		zap.String("user", username),
		zap.String("namespace", namespace),
		zap.String("image", req.Image),
		zap.Int("gpuCount", req.GPUCount),
		zap.String("gpuType", req.GPUType),
		zap.Int("ttlHours", req.TTLHours))

	// 检查配额
	if err := h.checkQuota(ctx, username, req.GPUCount); err != nil {
		h.log.Warn("Quota exceeded",
			zap.String("user", username),
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
	if err := h.k8sClient.EnsurePVC(ctx, namespace, username, storageClass, storageSize); err != nil {
		h.log.Error("Failed to create PVC",
			zap.String("namespace", namespace),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建存储失败: %v", err)})
		return
	}

	// 生成 Pod 名称和 SSH 端口
	podName := k8s.GeneratePodName(username)
	sshPort := k8s.GenerateSSHPort(username)
	expiresAt := time.Now().Add(time.Duration(req.TTLHours) * time.Hour)

	// 创建 Pod
	spec := &k8s.PodSpec{
		Name:       podName,
		Namespace:  namespace,
		Username:   username,
		Image:      req.Image,
		GPUCount:   req.GPUCount,
		GPUType:    req.GPUType,
		SSHPort:    sshPort,
		Password:   "genetpod2024",
		TTLHours:   req.TTLHours,
		ExpiresAt:  expiresAt,
		HTTPProxy:  h.config.Proxy.HTTPProxy,
		HTTPSProxy: h.config.Proxy.HTTPSProxy,
		NoProxy:    h.config.Proxy.NoProxy,
	}

	h.log.Debug("Creating pod resource",
		zap.String("podName", podName),
		zap.Int32("sshPort", sshPort))

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
		zap.Int("gpuCount", req.GPUCount),
		zap.Int32("sshPort", sshPort),
		zap.Time("expiresAt", expiresAt))

	c.JSON(http.StatusCreated, gin.H{
		"message": "Pod 创建成功",
		"id":      podName,
		"name":    podName,
		"sshPort": sshPort,
	})
}

// GetPod 获取 Pod 详情
func (h *PodHandler) GetPod(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	podID := c.Param("id")
	namespace := k8s.GetNamespaceForUser(username)
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
	expiresAt, _ := time.Parse(time.RFC3339, pod.Annotations["genet.io/expires-at"])

	// 获取节点 IP
	nodeIP := h.getNodeIP(ctx, pod.Spec.NodeName)

	// 获取 SSH 端口和密码
	sshPort := int32(0)
	if portStr, ok := pod.Annotations["genet.io/ssh-port"]; ok {
		if port, err := strconv.ParseInt(portStr, 10, 32); err == nil {
			sshPort = int32(port)
		}
	}
	password := pod.Annotations["genet.io/password"]

	// 构建连接信息
	connections := h.buildConnectionInfo(nodeIP, sshPort, password, pod.Name)

	response := models.PodResponse{
		ID:          pod.Name,
		Name:        pod.Name,
		Status:      h.getPodStatus(pod),
		Phase:       string(pod.Status.Phase),
		Image:       pod.Annotations["genet.io/image"],
		GPUType:     pod.Annotations["genet.io/gpu-type"],
		GPUCount:    gpuCount,
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
		NodeIP:      nodeIP,
		Connections: connections,
	}

	c.JSON(http.StatusOK, response)
}

// ExtendPod 延长 Pod TTL
func (h *PodHandler) ExtendPod(c *gin.Context) {
	var req models.ExtendPodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Warn("Invalid extend request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}

	username, _ := auth.GetUsername(c)
	podID := c.Param("id")
	namespace := k8s.GetNamespaceForUser(username)
	ctx := context.Background()

	h.log.Info("Extending pod TTL",
		zap.String("user", username),
		zap.String("podID", podID),
		zap.Int("hours", req.Hours))

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

	// 计算新的过期时间
	currentExpiresAt, _ := time.Parse(time.RFC3339, pod.Annotations["genet.io/expires-at"])
	newExpiresAt := currentExpiresAt.Add(time.Duration(req.Hours) * time.Hour)

	// 更新 Pod 注解
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations["genet.io/expires-at"] = newExpiresAt.Format(time.RFC3339)

	clientset := h.k8sClient.GetClientset()
	_, err = clientset.CoreV1().Pods(namespace).Update(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		h.log.Error("Failed to extend pod TTL",
			zap.String("user", username),
			zap.String("podID", podID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("延长时间失败: %v", err)})
		return
	}

	h.log.Info("Pod TTL extended",
		zap.String("user", username),
		zap.String("podID", podID),
		zap.Time("oldExpiresAt", currentExpiresAt),
		zap.Time("newExpiresAt", newExpiresAt))

	c.JSON(http.StatusOK, gin.H{
		"message":   "延长时间成功",
		"expiresAt": newExpiresAt.Format(time.RFC3339),
	})
}

// DeletePod 删除 Pod
func (h *PodHandler) DeletePod(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	podID := c.Param("id")
	namespace := k8s.GetNamespaceForUser(username)
	ctx := context.Background()

	h.log.Info("Deleting pod",
		zap.String("user", username),
		zap.String("podID", podID),
		zap.String("namespace", namespace))

	// 删除 Pod（保留 PVC）
	err := h.k8sClient.DeletePod(ctx, namespace, podID)
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

	c.JSON(http.StatusOK, gin.H{"message": "Pod 删除成功"})
}

// GetPodLogs 获取 Pod 日志
func (h *PodHandler) GetPodLogs(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	podID := c.Param("id")
	namespace := k8s.GetNamespaceForUser(username)
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
	podID := c.Param("id")
	namespace := k8s.GetNamespaceForUser(username)
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
	podID := c.Param("id")
	namespace := k8s.GetNamespaceForUser(username)
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
	podID := c.Param("id")
	namespace := k8s.GetNamespaceForUser(username)
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
	podID := c.Param("id")
	namespace := k8s.GetNamespaceForUser(username)

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
	podID := c.Param("id")
	namespace := k8s.GetNamespaceForUser(username)

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
func (h *PodHandler) checkQuota(ctx context.Context, username string, requestGPUCount int) error {
	namespace := k8s.GetNamespaceForUser(username)

	// 列出用户的 Pod
	pods, err := h.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		// 如果命名空间不存在，视为没有 Pod
		h.log.Debug("Namespace not exists, quota check passed",
			zap.String("user", username),
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
		zap.String("user", username),
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

// buildConnectionInfo 构建连接信息
func (h *PodHandler) buildConnectionInfo(nodeIP string, sshPort int32, password, podName string) models.ConnectionInfo {
	if nodeIP == "" || sshPort == 0 {
		return models.ConnectionInfo{}
	}

	sshCommand := fmt.Sprintf("ssh root@%s -p %d", nodeIP, sshPort)

	// VSCode Remote SSH URI
	// 注意：是否在新窗口打开取决于用户的 VSCode 设置 (window.openFoldersInNewWindow)
	vscodeURI := fmt.Sprintf("vscode://vscode-remote/ssh-remote+root@%s:%d/workspace", nodeIP, sshPort)

	// SSH URI - 某些系统会关联到默认 SSH 客户端（如 PuTTY、Termius 等）
	sshURI := fmt.Sprintf("ssh://root@%s:%d", nodeIP, sshPort)

	// Mac Terminal 命令
	macTerminalCmd := fmt.Sprintf("ssh root@%s -p %d", nodeIP, sshPort)

	// Windows Terminal 命令
	winTerminalCmd := fmt.Sprintf("ssh root@%s -p %d", nodeIP, sshPort)

	return models.ConnectionInfo{
		SSH: models.SSHConnection{
			Host:     nodeIP,
			Port:     sshPort,
			User:     "root",
			Password: password,
		},
		Apps: models.AppConnections{
			SSHCommand:     sshCommand,
			VSCodeURI:      vscodeURI,
			XshellURI:      sshURI, // ssh:// 协议，可能打开默认 SSH 客户端
			MacTerminalCmd: macTerminalCmd,
			WinTerminalCmd: winTerminalCmd,
		},
	}
}
