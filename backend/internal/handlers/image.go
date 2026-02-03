package handlers

import (
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
)

// ImageHandler 用户镜像处理器
type ImageHandler struct {
	k8sClient *k8s.Client
	config    *models.Config
	log       *zap.Logger
}

// NewImageHandler 创建用户镜像处理器
func NewImageHandler(k8sClient *k8s.Client, config *models.Config) *ImageHandler {
	return &ImageHandler{
		k8sClient: k8sClient,
		config:    config,
		log:       logger.Named("image-handler"),
	}
}

// ListUserImages 获取用户保存的镜像列表
func (h *ImageHandler) ListUserImages(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := c.Request.Context()

	imageList, err := h.k8sClient.GetUserImages(ctx, namespace)
	if err != nil {
		h.log.Error("Failed to get user images",
			zap.String("user", username),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取镜像列表失败"})
		return
	}

	// 按保存时间降序排列
	sort.Slice(imageList.Images, func(i, j int) bool {
		return imageList.Images[i].SavedAt.After(imageList.Images[j].SavedAt)
	})

	c.JSON(http.StatusOK, imageList)
}

// AddUserImage 添加用户镜像记录
func (h *ImageHandler) AddUserImage(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := c.Request.Context()

	var req models.AddUserImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请指定镜像名称"})
		return
	}

	image := &models.UserSavedImage{
		Image:       req.Image,
		Description: req.Description,
		SourcePod:   req.SourcePod,
		SavedAt:     time.Now(),
	}

	if err := h.k8sClient.SaveUserImage(ctx, namespace, image); err != nil {
		h.log.Error("Failed to save user image",
			zap.String("user", username),
			zap.String("image", req.Image),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存镜像记录失败"})
		return
	}

	h.log.Info("User image saved",
		zap.String("user", username),
		zap.String("image", req.Image))

	c.JSON(http.StatusOK, gin.H{"message": "镜像记录已保存"})
}

// DeleteUserImage 删除用户镜像记录
func (h *ImageHandler) DeleteUserImage(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	ctx := c.Request.Context()

	imageName := c.Query("image")
	if imageName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请指定要删除的镜像名称"})
		return
	}

	if err := h.k8sClient.DeleteUserImage(ctx, namespace, imageName); err != nil {
		h.log.Error("Failed to delete user image",
			zap.String("user", username),
			zap.String("image", imageName),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除镜像记录失败"})
		return
	}

	h.log.Info("User image deleted",
		zap.String("user", username),
		zap.String("image", imageName))

	c.JSON(http.StatusOK, gin.H{"message": "镜像记录已删除"})
}
