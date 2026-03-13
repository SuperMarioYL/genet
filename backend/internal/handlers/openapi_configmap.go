package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
)

func buildConfigMapSummary(cmName, namespace string, immutable bool, data map[string]string, binaryData map[string][]byte) models.OpenAPIConfigMapSummary {
	dataKeys := make([]string, 0, len(data))
	for key := range data {
		dataKeys = append(dataKeys, key)
	}
	sort.Strings(dataKeys)

	binaryKeys := make([]string, 0, len(binaryData))
	for key := range binaryData {
		binaryKeys = append(binaryKeys, key)
	}
	sort.Strings(binaryKeys)

	return models.OpenAPIConfigMapSummary{
		Name:           cmName,
		Namespace:      namespace,
		Immutable:      immutable,
		DataKeys:       dataKeys,
		BinaryDataKeys: binaryKeys,
	}
}

func buildConfigMapDetail(cm *models.OpenAPIConfigMapSummary, data map[string]string, binaryData map[string][]byte) models.OpenAPIConfigMapDetail {
	detail := models.OpenAPIConfigMapDetail{
		OpenAPIConfigMapSummary: *cm,
		Data:                    data,
		BinaryData:              map[string]string{},
	}
	for key, value := range binaryData {
		detail.BinaryData[key] = base64.StdEncoding.EncodeToString(value)
	}
	return detail
}

func (h *OpenAPIHandler) CreateConfigMap(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	var req models.OpenAPIConfigMapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的请求参数: %v", err)})
		return
	}
	if err := ValidateOpenAPIConfigMapRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	ctx := c.Request.Context()
	if err := h.k8sClient.EnsureNamespace(ctx, namespace); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to ensure namespace: %v", err)})
		return
	}

	configMap, err := k8s.BuildConfigMapFromOpenAPIRequest(namespace, ownerUser, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.k8sClient.CreateConfigMap(ctx, configMap)
	if err != nil {
		h.log.Error("Failed to create configmap", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create configmap: %v", err)})
		return
	}

	summary := buildConfigMapSummary(
		created.Name,
		created.Namespace,
		created.Immutable != nil && *created.Immutable,
		created.Data,
		created.BinaryData,
	)
	summary.CreatedAt = created.CreationTimestamp.Time

	c.JSON(http.StatusCreated, buildConfigMapDetail(&summary, created.Data, created.BinaryData))
}

func (h *OpenAPIHandler) ListConfigMaps(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	labelSelector := c.Query("labelSelector")
	list, err := h.k8sClient.ListConfigMaps(c.Request.Context(), namespace, openAPIOwnerLabelSelector(ownerUser, labelSelector))
	if err != nil {
		h.log.Error("Failed to list configmaps", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list configmaps: %v", err)})
		return
	}

	resp := models.OpenAPIConfigMapListResponse{
		ConfigMaps: make([]models.OpenAPIConfigMapSummary, 0, len(list.Items)),
	}
	for _, item := range list.Items {
		summary := buildConfigMapSummary(
			item.Name,
			item.Namespace,
			item.Immutable != nil && *item.Immutable,
			item.Data,
			item.BinaryData,
		)
		summary.CreatedAt = item.CreationTimestamp.Time
		resp.ConfigMaps = append(resp.ConfigMaps, summary)
	}

	c.JSON(http.StatusOK, resp)
}

func (h *OpenAPIHandler) GetConfigMap(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	cm, err := h.k8sClient.GetConfigMap(c.Request.Context(), openAPIOwnerNamespace(ownerUser), c.Param("name"))
	if err != nil || !isOpenAPIOwnedBy(cm.Labels, ownerUser) {
		c.JSON(http.StatusNotFound, gin.H{"error": "configmap not found"})
		return
	}

	summary := buildConfigMapSummary(
		cm.Name,
		cm.Namespace,
		cm.Immutable != nil && *cm.Immutable,
		cm.Data,
		cm.BinaryData,
	)
	summary.CreatedAt = cm.CreationTimestamp.Time

	c.JSON(http.StatusOK, buildConfigMapDetail(&summary, cm.Data, cm.BinaryData))
}

func (h *OpenAPIHandler) UpdateConfigMap(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	var req models.OpenAPIConfigMapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的请求参数: %v", err)})
		return
	}
	req.Name = c.Param("name")
	if err := ValidateOpenAPIConfigMapRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	ctx := c.Request.Context()
	existing, err := h.k8sClient.GetConfigMap(ctx, namespace, req.Name)
	if err != nil || !isOpenAPIOwnedBy(existing.Labels, ownerUser) {
		c.JSON(http.StatusNotFound, gin.H{"error": "configmap not found"})
		return
	}
	if existing.Immutable != nil && *existing.Immutable {
		c.JSON(http.StatusConflict, gin.H{"error": "configmap is immutable"})
		return
	}

	configMap, err := k8s.BuildConfigMapFromOpenAPIRequest(namespace, ownerUser, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	configMap.ResourceVersion = existing.ResourceVersion

	updated, err := h.k8sClient.UpdateConfigMap(ctx, configMap)
	if err != nil {
		h.log.Error("Failed to update configmap", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update configmap: %v", err)})
		return
	}

	summary := buildConfigMapSummary(
		updated.Name,
		updated.Namespace,
		updated.Immutable != nil && *updated.Immutable,
		updated.Data,
		updated.BinaryData,
	)
	summary.CreatedAt = updated.CreationTimestamp.Time

	c.JSON(http.StatusOK, buildConfigMapDetail(&summary, updated.Data, updated.BinaryData))
}

func (h *OpenAPIHandler) DeleteConfigMap(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	name := c.Param("name")
	cm, err := h.k8sClient.GetConfigMap(c.Request.Context(), namespace, name)
	if err != nil || !isOpenAPIOwnedBy(cm.Labels, ownerUser) {
		c.JSON(http.StatusNotFound, gin.H{"error": "configmap not found"})
		return
	}

	if err := h.k8sClient.DeleteConfigMap(c.Request.Context(), namespace, name); err != nil {
		h.log.Error("Failed to delete configmap", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete configmap: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "configmap deleted"})
}
