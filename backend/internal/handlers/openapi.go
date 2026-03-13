package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// OpenAPIHandler Open API 处理器
type OpenAPIHandler struct {
	k8sClient  *k8s.Client
	podHandler *PodHandler
	config     *models.Config
	log        *zap.Logger
}

// NewOpenAPIHandler 创建 Open API 处理器
func NewOpenAPIHandler(k8sClient *k8s.Client, config *models.Config) *OpenAPIHandler {
	return &OpenAPIHandler{
		k8sClient:  k8sClient,
		podHandler: NewPodHandler(k8sClient, nil, config),
		config:     config,
		log:        logger.Named("openapi"),
	}
}

// bindYAML 从请求体读取 YAML 并反序列化到目标对象
func (h *OpenAPIHandler) bindYAML(c *gin.Context, obj interface{}) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}
	if len(body) == 0 {
		return fmt.Errorf("empty request body")
	}
	if err := yaml.Unmarshal(body, obj); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	return nil
}

// enforceNamespace 强制覆写 namespace 为配置值
func (h *OpenAPIHandler) enforceNamespace(meta *metav1.ObjectMeta) {
	meta.Namespace = h.config.OpenAPI.Namespace
}

// getNamespace 返回配置的固定 namespace
func (h *OpenAPIHandler) getNamespace() string {
	return h.config.OpenAPI.Namespace
}

// ensureLabels 确保资源带有 open-api 标识 label
func (h *OpenAPIHandler) ensureLabels(meta *metav1.ObjectMeta) {
	if meta.Labels == nil {
		meta.Labels = make(map[string]string)
	}
	meta.Labels["genet.io/open-api"] = "true"
	meta.Labels["genet.io/managed"] = "true"
}

// openAPILabelSelector 构建包含 open-api 过滤的 labelSelector
func (h *OpenAPIHandler) openAPILabelSelector(extra string) string {
	base := "genet.io/open-api=true"
	if extra != "" {
		return extra + "," + base
	}
	return base
}

// ==================== Pod CRUD ====================

func applyOpenAPIOwnerUserContext(c *gin.Context) (string, bool) {
	ownerUser := strings.TrimSpace(c.GetString("openapiOwnerUser"))
	if ownerUser == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: api key must bind ownerUser"})
		c.Abort()
		return "", false
	}
	// Reuse existing PodHandler flow by injecting auth context from API key binding.
	c.Set("username", ownerUser)
	c.Set("email", "")
	c.Set("authenticated", true)
	c.Set("authMethod", "apikey")
	return k8s.GetUserIdentifier(ownerUser, ""), true
}

func deriveCustomNameForUpdate(podID, userIdentifier string) string {
	if podID == "" || userIdentifier == "" {
		return ""
	}
	prefix := fmt.Sprintf("pod-%s-", userIdentifier)
	if !strings.HasPrefix(podID, prefix) {
		return ""
	}
	suffix := strings.TrimSpace(strings.TrimPrefix(podID, prefix))
	return suffix
}

func openAPIOwnerLabelSelector(ownerUser, extra string) string {
	base := fmt.Sprintf("genet.io/open-api=true,genet.io/openapi-owner=%s", ownerUser)
	if extra != "" {
		return extra + "," + base
	}
	return base
}

func isOpenAPIOwnedBy(labels map[string]string, ownerUser string) bool {
	return labels["genet.io/openapi-owner"] == ownerUser
}

// CreatePod 创建 Pod（JSON 格式，字段与 UI 创建 Pod 一致）
func (h *OpenAPIHandler) CreatePod(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.podHandler.CreatePod(c)
}

// ListPods 列出 Pod
func (h *OpenAPIHandler) ListPods(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.podHandler.ListPods(c)
}

// GetPod 获取 Pod
func (h *OpenAPIHandler) GetPod(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.podHandler.GetPod(c)
}

// UpdatePod 更新 Pod（语义：删除旧 Pod 后按 JSON 请求重建）
func (h *OpenAPIHandler) UpdatePod(c *gin.Context) {
	userIdentifier, ok := applyOpenAPIOwnerUserContext(c)
	if !ok {
		return
	}

	podID := c.Param("id")
	if podID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pod id is required"})
		return
	}

	var req models.PodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的请求参数: %v", err)})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = deriveCustomNameForUpdate(podID, userIdentifier)
	}

	ctx := c.Request.Context()
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)

	_, err := h.k8sClient.GetPod(ctx, namespace, podID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pod 不存在"})
		return
	}

	commitStatus, err := h.k8sClient.GetCommitJobStatus(ctx, namespace, podID)
	if err == nil && commitStatus != nil {
		if commitStatus.Status == "Running" || commitStatus.Status == "Pending" {
			c.JSON(http.StatusConflict, gin.H{
				"error": fmt.Sprintf("Pod 有正在进行的镜像保存任务（%s），请等待完成后再更新", commitStatus.Status),
			})
			return
		}
	}

	if err := h.k8sClient.DeletePod(ctx, namespace, podID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("删除旧 Pod 失败: %v", err)})
		return
	}
	if err := h.k8sClient.DeletePodScopedPVCs(ctx, namespace, userIdentifier, podID); err != nil {
		h.log.Warn("Failed to delete some scope=pod PVCs while updating pod",
			zap.String("userIdentifier", userIdentifier),
			zap.String("podID", podID),
			zap.Error(err))
	}

	body, err := json.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "构建更新请求失败"})
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.podHandler.CreatePod(c)
}

// DeletePod 删除 Pod
func (h *OpenAPIHandler) DeletePod(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.podHandler.DeletePod(c)
}

// ==================== Job CRUD ====================

// CreateJob 创建 Job
func (h *OpenAPIHandler) CreateJob(c *gin.Context) {
	var job batchv1.Job
	if err := h.bindYAML(c, &job); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.enforceNamespace(&job.ObjectMeta)
	h.ensureLabels(&job.ObjectMeta)
	if owner := c.GetString("openapiOwnerUser"); owner != "" {
		job.Labels["genet.io/openapi-owner"] = owner
	}
	// 清理 Pod template 中的 namespace（安全起见）
	job.Spec.Template.ObjectMeta.Namespace = ""

	h.log.Info("Creating job via Open API",
		zap.String("name", job.Name),
		zap.String("namespace", job.Namespace))

	ctx := c.Request.Context()

	if err := h.k8sClient.EnsureNamespace(ctx, h.getNamespace()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to ensure namespace: %v", err)})
		return
	}

	clientset := h.k8sClient.GetClientset()
	created, err := clientset.BatchV1().Jobs(h.getNamespace()).Create(ctx, &job, metav1.CreateOptions{})
	if err != nil {
		h.log.Error("Failed to create job", zap.Error(err))
		if errors.IsAlreadyExists(err) {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("job %q already exists", job.Name)})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create job: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, created)
}

// ListJobs 列出 Job
func (h *OpenAPIHandler) ListJobs(c *gin.Context) {
	ctx := c.Request.Context()
	labelSelector := h.openAPILabelSelector(c.Query("labelSelector"))

	clientset := h.k8sClient.GetClientset()
	jobs, err := clientset.BatchV1().Jobs(h.getNamespace()).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		h.log.Error("Failed to list jobs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list jobs: %v", err)})
		return
	}

	c.JSON(http.StatusOK, jobs)
}

// GetJob 获取 Job
func (h *OpenAPIHandler) GetJob(c *gin.Context) {
	name := c.Param("name")
	ctx := c.Request.Context()

	clientset := h.k8sClient.GetClientset()
	job, err := clientset.BatchV1().Jobs(h.getNamespace()).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		h.log.Error("Failed to get job", zap.String("name", name), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get job: %v", err)})
		return
	}

	c.JSON(http.StatusOK, job)
}

// UpdateJob 更新 Job
func (h *OpenAPIHandler) UpdateJob(c *gin.Context) {
	name := c.Param("name")

	var job batchv1.Job
	if err := h.bindYAML(c, &job); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.enforceNamespace(&job.ObjectMeta)
	job.Name = name

	ctx := c.Request.Context()
	clientset := h.k8sClient.GetClientset()
	updated, err := clientset.BatchV1().Jobs(h.getNamespace()).Update(ctx, &job, metav1.UpdateOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		h.log.Error("Failed to update job", zap.String("name", name), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update job: %v", err)})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteJob 删除 Job
func (h *OpenAPIHandler) DeleteJob(c *gin.Context) {
	name := c.Param("name")
	ctx := c.Request.Context()

	clientset := h.k8sClient.GetClientset()
	propagation := metav1.DeletePropagationBackground
	err := clientset.BatchV1().Jobs(h.getNamespace()).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		h.log.Error("Failed to delete job", zap.String("name", name), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete job: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "job deleted"})
}
