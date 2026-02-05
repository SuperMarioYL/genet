package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/models"
	"github.com/uc-package/genet/internal/registry"
	"go.uber.org/zap"
)

// RegistryHandler Registry API 处理器
type RegistryHandler struct {
	config   *models.Config
	client   registry.Client
	log      *zap.Logger
}

// NewRegistryHandler 创建 Registry 处理器
func NewRegistryHandler(config *models.Config, log *zap.Logger) (*RegistryHandler, error) {
	client, err := registry.NewClient(&config.Registry)
	if err != nil {
		return nil, err
	}

	return &RegistryHandler{
		config: config,
		client: client,
		log:    log,
	}, nil
}

// SearchImagesResponse 镜像搜索响应
type SearchImagesResponse struct {
	Images []registry.ImageInfo `json:"images"`
}

// SearchImages 搜索镜像
// GET /api/registry/images?keyword=xxx&limit=20
func (h *RegistryHandler) SearchImages(c *gin.Context) {
	keyword := c.Query("keyword")
	limitStr := c.DefaultQuery("limit", "20")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// 检查 registry 是否已配置
	if !h.client.IsConfigured() {
		c.JSON(http.StatusOK, SearchImagesResponse{Images: []registry.ImageInfo{}})
		return
	}

	ctx := c.Request.Context()
	images, err := h.client.SearchImages(ctx, keyword, limit)
	if err != nil {
		h.log.Warn("Failed to search images from registry",
			zap.String("keyword", keyword),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "搜索镜像失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, SearchImagesResponse{Images: images})
}

// GetImageTagsResponse 镜像 tags 响应
type GetImageTagsResponse struct {
	Tags []string `json:"tags"`
}

// GetImageTags 获取镜像的 tags，支持按 platform 过滤
// GET /api/registry/tags?image=xxx&platform=amd64
func (h *RegistryHandler) GetImageTags(c *gin.Context) {
	imageName := c.Query("image")
	if imageName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请指定镜像名称"})
		return
	}

	platform := c.Query("platform")

	// 检查 registry 是否已配置
	if !h.client.IsConfigured() {
		c.JSON(http.StatusOK, GetImageTagsResponse{Tags: []string{}})
		return
	}

	ctx := c.Request.Context()
	tags, err := h.client.GetImageTags(ctx, imageName, platform)
	if err != nil {
		h.log.Warn("Failed to get image tags from registry",
			zap.String("image", imageName),
			zap.String("platform", platform),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取镜像标签失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, GetImageTagsResponse{Tags: tags})
}
