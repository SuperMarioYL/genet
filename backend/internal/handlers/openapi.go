package handlers

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// OpenAPIHandler Open API 处理器
type OpenAPIHandler struct {
	k8sClient *k8s.Client
	config    *models.Config
	log       *zap.Logger
}

// NewOpenAPIHandler 创建 Open API 处理器
func NewOpenAPIHandler(k8sClient *k8s.Client, config *models.Config) *OpenAPIHandler {
	return &OpenAPIHandler{
		k8sClient: k8sClient,
		config:    config,
		log:       logger.Named("openapi"),
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

// CreatePod 创建 Pod
func (h *OpenAPIHandler) CreatePod(c *gin.Context) {
	var pod corev1.Pod
	if err := h.bindYAML(c, &pod); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.enforceNamespace(&pod.ObjectMeta)
	h.ensureLabels(&pod.ObjectMeta)

	h.log.Info("Creating pod via Open API",
		zap.String("name", pod.Name),
		zap.String("namespace", pod.Namespace))

	ctx := c.Request.Context()

	if err := h.k8sClient.EnsureNamespace(ctx, h.getNamespace()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to ensure namespace: %v", err)})
		return
	}

	clientset := h.k8sClient.GetClientset()
	created, err := clientset.CoreV1().Pods(h.getNamespace()).Create(ctx, &pod, metav1.CreateOptions{})
	if err != nil {
		h.log.Error("Failed to create pod", zap.Error(err))
		if errors.IsAlreadyExists(err) {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("pod %q already exists", pod.Name)})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create pod: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, created)
}

// ListPods 列出 Pod
func (h *OpenAPIHandler) ListPods(c *gin.Context) {
	ctx := c.Request.Context()
	labelSelector := h.openAPILabelSelector(c.Query("labelSelector"))

	clientset := h.k8sClient.GetClientset()
	pods, err := clientset.CoreV1().Pods(h.getNamespace()).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		h.log.Error("Failed to list pods", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list pods: %v", err)})
		return
	}

	c.JSON(http.StatusOK, pods)
}

// GetPod 获取 Pod
func (h *OpenAPIHandler) GetPod(c *gin.Context) {
	name := c.Param("name")
	ctx := c.Request.Context()

	clientset := h.k8sClient.GetClientset()
	pod, err := clientset.CoreV1().Pods(h.getNamespace()).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "pod not found"})
			return
		}
		h.log.Error("Failed to get pod", zap.String("name", name), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get pod: %v", err)})
		return
	}

	c.JSON(http.StatusOK, pod)
}

// UpdatePod 更新 Pod
func (h *OpenAPIHandler) UpdatePod(c *gin.Context) {
	name := c.Param("name")

	var pod corev1.Pod
	if err := h.bindYAML(c, &pod); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.enforceNamespace(&pod.ObjectMeta)
	pod.Name = name

	ctx := c.Request.Context()
	clientset := h.k8sClient.GetClientset()
	updated, err := clientset.CoreV1().Pods(h.getNamespace()).Update(ctx, &pod, metav1.UpdateOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "pod not found"})
			return
		}
		h.log.Error("Failed to update pod", zap.String("name", name), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update pod: %v", err)})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeletePod 删除 Pod
func (h *OpenAPIHandler) DeletePod(c *gin.Context) {
	name := c.Param("name")
	ctx := c.Request.Context()

	clientset := h.k8sClient.GetClientset()
	err := clientset.CoreV1().Pods(h.getNamespace()).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "pod not found"})
			return
		}
		h.log.Error("Failed to delete pod", zap.String("name", name), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete pod: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "pod deleted"})
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
