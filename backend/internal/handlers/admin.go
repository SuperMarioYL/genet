package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
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

// AdminHandler 管理接口处理器
type AdminHandler struct {
	config    *models.Config
	k8sClient *k8s.Client
	log       *zap.Logger
}

// AdminMeResponse 管理员身份检查响应
type AdminMeResponse struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	IsAdmin  bool   `json:"isAdmin"`
}

// AdminAPIKeyItem 管理页显示的 API Key 条目
type AdminAPIKeyItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	OwnerUser  string     `json:"ownerUser"`
	Scope      string     `json:"scope"`
	Enabled    bool       `json:"enabled"`
	KeyPreview string     `json:"keyPreview,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	CreatedBy  string     `json:"createdBy"`
}

// AdminAPIKeyListResponse API Key 列表响应
type AdminAPIKeyListResponse struct {
	Items []AdminAPIKeyItem `json:"items"`
}

type AdminOverviewResponse struct {
	NodeSummary AdminPoolSummary `json:"nodeSummary"`
	UserSummary AdminPoolSummary `json:"userSummary"`
}

type AdminPoolSummary struct {
	Shared    int `json:"shared"`
	Exclusive int `json:"exclusive"`
}

type AdminNodePoolItem struct {
	NodeName string `json:"nodeName"`
	NodeIP   string `json:"nodeIP"`
	PoolType string `json:"poolType"`
}

type AdminNodePoolListResponse struct {
	Nodes []AdminNodePoolItem `json:"nodes"`
}

type AdminUserPoolItem struct {
	Username  string     `json:"username"`
	Email     string     `json:"email,omitempty"`
	PoolType  string     `json:"poolType"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	UpdatedBy string     `json:"updatedBy,omitempty"`
}

type AdminUserPoolListResponse struct {
	Users []AdminUserPoolItem `json:"users"`
}

type UpdatePoolTypeRequest struct {
	PoolType string `json:"poolType" binding:"required"`
}

// CreateAdminAPIKeyRequest 创建 API Key 请求
type CreateAdminAPIKeyRequest struct {
	Name      string     `json:"name" binding:"required"`
	OwnerUser string     `json:"ownerUser" binding:"required"`
	Scope     string     `json:"scope,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// UpdateAdminAPIKeyRequest 更新 API Key 请求
type UpdateAdminAPIKeyRequest struct {
	Name      *string    `json:"name,omitempty"`
	OwnerUser *string    `json:"ownerUser,omitempty"`
	Scope     *string    `json:"scope,omitempty"`
	Enabled   *bool      `json:"enabled,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// NewAdminHandler 创建管理处理器
func NewAdminHandler(config *models.Config, k8sClient *k8s.Client) *AdminHandler {
	return &AdminHandler{
		config:    config,
		k8sClient: k8sClient,
		log:       logger.Named("admin"),
	}
}

// GetMe 返回当前用户的管理员状态。
func (h *AdminHandler) GetMe(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)

	c.JSON(http.StatusOK, AdminMeResponse{
		Username: username,
		Email:    email,
		IsAdmin:  auth.IsAdmin(h.config, username, email),
	})
}

func (h *AdminHandler) GetOverview(c *gin.Context) {
	nodes, users, err := h.listAdminPoolState(c.Request.Context())
	if err != nil {
		h.log.Error("Failed to load admin overview", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load admin overview"})
		return
	}

	resp := AdminOverviewResponse{}
	for _, node := range nodes {
		if node.PoolType == k8s.UserPoolTypeExclusive {
			resp.NodeSummary.Exclusive++
		} else {
			resp.NodeSummary.Shared++
		}
	}
	for _, user := range users {
		if user.PoolType == k8s.UserPoolTypeExclusive {
			resp.UserSummary.Exclusive++
		} else {
			resp.UserSummary.Shared++
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (h *AdminHandler) ListNodePools(c *gin.Context) {
	nodes, _, err := h.listAdminPoolState(c.Request.Context())
	if err != nil {
		h.log.Error("Failed to list node pools", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list node pools"})
		return
	}
	c.JSON(http.StatusOK, AdminNodePoolListResponse{Nodes: nodes})
}

func (h *AdminHandler) UpdateNodePool(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	var req UpdatePoolTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}
	poolType := strings.TrimSpace(req.PoolType)
	if poolType != k8s.UserPoolTypeShared && poolType != k8s.UserPoolTypeExclusive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid poolType, must be shared or exclusive"})
		return
	}

	nodeName := strings.TrimSpace(c.Param("name"))
	node, err := h.k8sClient.GetClientset().CoreV1().Nodes().Get(c.Request.Context(), nodeName, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	labelKey := defaultNonSharedLabelKey
	labelValue := defaultNonSharedLabelValue
	if h.config != nil {
		if v := strings.TrimSpace(h.config.GPU.NodePool.NonSharedLabelKey); v != "" {
			labelKey = v
		}
		if v := strings.TrimSpace(h.config.GPU.NodePool.NonSharedLabelValue); v != "" {
			labelValue = v
		}
	}
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	if poolType == k8s.UserPoolTypeExclusive {
		node.Labels[labelKey] = labelValue
	} else {
		delete(node.Labels, labelKey)
	}

	updated, err := h.k8sClient.GetClientset().CoreV1().Nodes().Update(c.Request.Context(), node, metav1.UpdateOptions{})
	if err != nil {
		h.log.Error("Failed to update node pool", zap.String("node", nodeName), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update node pool"})
		return
	}

	c.JSON(http.StatusOK, AdminNodePoolItem{
		NodeName: updated.Name,
		NodeIP:   adminNodeInternalIP(*updated),
		PoolType: getNodePoolType(*updated, h.config),
	})
}

func (h *AdminHandler) ListUserPools(c *gin.Context) {
	_, users, err := h.listAdminPoolState(c.Request.Context())
	if err != nil {
		h.log.Error("Failed to list user pools", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list user pools"})
		return
	}
	c.JSON(http.StatusOK, AdminUserPoolListResponse{Users: users})
}

func (h *AdminHandler) UpdateUserPool(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	var req UpdatePoolTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}
	poolType := strings.TrimSpace(req.PoolType)
	if poolType != k8s.UserPoolTypeShared && poolType != k8s.UserPoolTypeExclusive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid poolType, must be shared or exclusive"})
		return
	}

	username := strings.TrimSpace(c.Param("username"))
	operator, _ := auth.GetUsername(c)
	if operator == "" {
		operator, _ = auth.GetEmail(c)
	}
	record := k8s.UserPoolBindingRecord{
		Username:  username,
		PoolType:  poolType,
		UpdatedAt: time.Now().UTC(),
		UpdatedBy: operator,
	}
	if err := h.k8sClient.UpsertUserPoolBinding(c.Request.Context(), record); err != nil {
		h.log.Error("Failed to update user pool", zap.String("username", username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user pool"})
		return
	}

	c.JSON(http.StatusOK, AdminUserPoolItem{
		Username:  record.Username,
		PoolType:  record.PoolType,
		UpdatedAt: &record.UpdatedAt,
		UpdatedBy: record.UpdatedBy,
	})
}

func (h *AdminHandler) DeleteUser(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	username := strings.TrimSpace(c.Param("username"))
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	namespace := k8s.GetNamespaceForUserIdentifier(username)
	if err := h.k8sClient.DeleteUserPoolBinding(c.Request.Context(), username); err != nil {
		h.log.Error("Failed to delete user pool binding", zap.String("username", username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user pool binding"})
		return
	}
	if err := h.k8sClient.ForceDeleteNamespace(c.Request.Context(), namespace); err != nil {
		h.log.Error("Failed to delete user namespace", zap.String("username", username), zap.String("namespace", namespace), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user resources"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "user deletion requested",
		"username":  username,
		"namespace": namespace,
	})
}

// ListAPIKeys 列出所有管理 API Key。
func (h *AdminHandler) ListAPIKeys(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	records, err := h.k8sClient.ListOpenAPIKeys(c.Request.Context())
	if err != nil {
		h.log.Error("Failed to list OpenAPI keys", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list api keys"})
		return
	}

	items := make([]AdminAPIKeyItem, 0, len(records))
	for _, rec := range records {
		items = append(items, toAdminAPIKeyItem(rec))
	}

	c.JSON(http.StatusOK, AdminAPIKeyListResponse{Items: items})
}

// CreateAPIKey 创建一个新的管理 API Key。
func (h *AdminHandler) CreateAPIKey(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	var req CreateAdminAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.OwnerUser = strings.TrimSpace(req.OwnerUser)
	scope := normalizeAPIKeyScope(req.Scope)
	if req.Name == "" || req.OwnerUser == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and ownerUser are required"})
		return
	}
	if !isValidAPIKeyScope(scope) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope, must be read or write"})
		return
	}

	plaintextKey, err := generateOpenAPIPlaintextKey()
	if err != nil {
		h.log.Error("Failed to generate plaintext key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate api key"})
		return
	}

	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	createdBy := strings.TrimSpace(username)
	if createdBy == "" {
		createdBy = strings.TrimSpace(email)
	}

	now := time.Now().UTC()
	rec := models.APIKeyRecord{
		ID:         generateOpenAPIRecordID(now),
		Name:       req.Name,
		OwnerUser:  req.OwnerUser,
		Scope:      scope,
		Enabled:    true,
		KeyHash:    k8s.HashOpenAPIKey(plaintextKey),
		KeyPreview: buildOpenAPIKeyPreview(plaintextKey),
		ExpiresAt:  req.ExpiresAt,
		CreatedAt:  now,
		UpdatedAt:  now,
		CreatedBy:  createdBy,
	}

	if err := h.k8sClient.CreateOpenAPIKey(c.Request.Context(), rec); err != nil {
		h.log.Error("Failed to create api key record", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create api key"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"item":         toAdminAPIKeyItem(rec),
		"plaintextKey": plaintextKey,
	})
}

// UpdateAPIKey 更新 API Key 元数据。
func (h *AdminHandler) UpdateAPIKey(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key id"})
		return
	}

	var req UpdateAdminAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request: %v", err)})
		return
	}

	records, err := h.k8sClient.ListOpenAPIKeys(c.Request.Context())
	if err != nil {
		h.log.Error("Failed to list keys before update", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update api key"})
		return
	}

	var target *models.APIKeyRecord
	for i := range records {
		if records[i].ID == id {
			target = &records[i]
			break
		}
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
		return
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty"})
			return
		}
		target.Name = name
	}
	if req.OwnerUser != nil {
		owner := strings.TrimSpace(*req.OwnerUser)
		if owner == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ownerUser cannot be empty"})
			return
		}
		target.OwnerUser = owner
	}
	if req.Scope != nil {
		scope := normalizeAPIKeyScope(*req.Scope)
		if !isValidAPIKeyScope(scope) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope, must be read or write"})
			return
		}
		target.Scope = scope
	}
	if req.Enabled != nil {
		target.Enabled = *req.Enabled
	}
	if req.ExpiresAt != nil {
		target.ExpiresAt = req.ExpiresAt
	}
	target.UpdatedAt = time.Now().UTC()

	if err := h.k8sClient.UpdateOpenAPIKey(c.Request.Context(), *target); err != nil {
		if errors.Is(err, k8s.ErrOpenAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
			return
		}
		h.log.Error("Failed to update api key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update api key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"item": toAdminAPIKeyItem(*target)})
}

// DeleteAPIKey 删除 API Key。
func (h *AdminHandler) DeleteAPIKey(c *gin.Context) {
	if h.k8sClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "k8s client is not initialized"})
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing key id"})
		return
	}

	if err := h.k8sClient.DeleteOpenAPIKey(c.Request.Context(), id); err != nil {
		if errors.Is(err, k8s.ErrOpenAPIKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
			return
		}
		h.log.Error("Failed to delete api key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete api key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func normalizeAPIKeyScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return models.APIKeyScopeRead
	}
	return scope
}

func isValidAPIKeyScope(scope string) bool {
	return scope == models.APIKeyScopeRead || scope == models.APIKeyScopeWrite
}

func toAdminAPIKeyItem(rec models.APIKeyRecord) AdminAPIKeyItem {
	return AdminAPIKeyItem{
		ID:         rec.ID,
		Name:       rec.Name,
		OwnerUser:  rec.OwnerUser,
		Scope:      rec.Scope,
		Enabled:    rec.Enabled,
		KeyPreview: rec.KeyPreview,
		ExpiresAt:  rec.ExpiresAt,
		CreatedAt:  rec.CreatedAt,
		UpdatedAt:  rec.UpdatedAt,
		CreatedBy:  rec.CreatedBy,
	}
}

func generateOpenAPIPlaintextKey() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "gk_" + hex.EncodeToString(buf), nil
}

func generateOpenAPIRecordID(now time.Time) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("ak_%d", now.UnixNano())
	}
	return fmt.Sprintf("ak_%d_%s", now.UnixNano(), hex.EncodeToString(buf))
}

func buildOpenAPIKeyPreview(plaintext string) string {
	if len(plaintext) <= 12 {
		return plaintext
	}
	return plaintext[:12] + "..."
}

func (h *AdminHandler) listAdminPoolState(ctx context.Context) ([]AdminNodePoolItem, []AdminUserPoolItem, error) {
	if h.k8sClient == nil {
		return nil, nil, fmt.Errorf("k8s client is not initialized")
	}

	nodes, err := h.k8sClient.GetClientset().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}
	acceleratorTypes := h.config.GetAcceleratorTypes()
	nodeItems := make([]AdminNodePoolItem, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		if !adminNodeHasAccelerator(node, acceleratorTypes) {
			continue
		}
		nodeItems = append(nodeItems, AdminNodePoolItem{
			NodeName: node.Name,
			NodeIP:   adminNodeInternalIP(node),
			PoolType: getNodePoolType(node, h.config),
		})
	}
	sort.Slice(nodeItems, func(i, j int) bool {
		return nodeItems[i].NodeName < nodeItems[j].NodeName
	})

	records, err := h.k8sClient.ListUserPoolBindings(ctx)
	if err != nil {
		return nil, nil, err
	}
	userMap := map[string]AdminUserPoolItem{}
	for _, rec := range records {
		recCopy := rec
		userMap[rec.Username] = AdminUserPoolItem{
			Username:  rec.Username,
			PoolType:  rec.PoolType,
			UpdatedAt: &recCopy.UpdatedAt,
			UpdatedBy: rec.UpdatedBy,
		}
	}

	pods, err := h.k8sClient.GetClientset().CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}
	for _, pod := range pods.Items {
		adminMergeUserPoolItem(userMap, pod.Labels["genet.io/user"], pod.Annotations["genet.io/email"])
	}

	deployments, err := h.k8sClient.GetClientset().AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}
	for _, deployment := range deployments.Items {
		adminMergeUserPoolItem(userMap, deployment.Labels["genet.io/user"], deployment.Annotations["genet.io/email"])
	}

	statefulSets, err := h.k8sClient.GetClientset().AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}
	for _, statefulSet := range statefulSets.Items {
		adminMergeUserPoolItem(userMap, statefulSet.Labels["genet.io/user"], statefulSet.Annotations["genet.io/email"])
	}

	userItems := make([]AdminUserPoolItem, 0, len(userMap))
	for _, item := range userMap {
		if item.PoolType == "" {
			item.PoolType = k8s.UserPoolTypeShared
		}
		userItems = append(userItems, item)
	}
	sort.Slice(userItems, func(i, j int) bool {
		return userItems[i].Username < userItems[j].Username
	})

	return nodeItems, userItems, nil
}

func adminNodeInternalIP(node corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

func adminNodeHasAccelerator(node corev1.Node, acceleratorTypes []models.AcceleratorType) bool {
	for _, accType := range acceleratorTypes {
		resourceName := corev1.ResourceName(strings.TrimSpace(accType.ResourceName))
		if resourceName == "" {
			continue
		}
		if quantity, ok := node.Status.Capacity[resourceName]; ok && quantity.Value() > 0 {
			return true
		}
		if quantity, ok := node.Status.Allocatable[resourceName]; ok && quantity.Value() > 0 {
			return true
		}
	}
	return false
}

func adminMergeUserPoolItem(userMap map[string]AdminUserPoolItem, username, email string) {
	username = strings.TrimSpace(username)
	if username == "" {
		return
	}

	item := userMap[username]
	item.Username = username
	if item.PoolType == "" {
		item.PoolType = k8s.UserPoolTypeShared
	}
	if item.Email == "" {
		item.Email = strings.TrimSpace(email)
	}
	userMap[username] = item
}
