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
)

// OpenAPIHandler Open API 处理器
type OpenAPIHandler struct {
	k8sClient          *k8s.Client
	podHandler         *PodHandler
	deploymentHandler  *DeploymentHandler
	statefulSetHandler *StatefulSetHandler
	config             *models.Config
	log                *zap.Logger
}

// NewOpenAPIHandler 创建 Open API 处理器
func NewOpenAPIHandler(k8sClient *k8s.Client, config *models.Config) *OpenAPIHandler {
	return &OpenAPIHandler{
		k8sClient:          k8sClient,
		podHandler:         NewPodHandler(k8sClient, nil, config),
		deploymentHandler:  NewDeploymentHandler(k8sClient, config),
		statefulSetHandler: NewStatefulSetHandler(k8sClient, config),
		config:             config,
		log:                logger.Named("openapi"),
	}
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

func (h *OpenAPIHandler) CreateStatefulSet(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.statefulSetHandler.CreateStatefulSet(c)
}

func (h *OpenAPIHandler) CreateDeployment(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.deploymentHandler.CreateDeployment(c)
}

func (h *OpenAPIHandler) ListDeployments(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.deploymentHandler.ListDeployments(c)
}

func (h *OpenAPIHandler) GetDeployment(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.deploymentHandler.GetDeployment(c)
}

func (h *OpenAPIHandler) DeleteDeployment(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.deploymentHandler.DeleteDeployment(c)
}

func (h *OpenAPIHandler) ListStatefulSets(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.statefulSetHandler.ListStatefulSets(c)
}

func (h *OpenAPIHandler) GetStatefulSet(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.statefulSetHandler.GetStatefulSet(c)
}

func (h *OpenAPIHandler) DeleteStatefulSet(c *gin.Context) {
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}
	h.statefulSetHandler.DeleteStatefulSet(c)
}
