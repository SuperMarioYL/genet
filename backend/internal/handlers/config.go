package handlers

import (
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
)

// ConfigHandler 配置处理器
type ConfigHandler struct {
	config    *models.Config
	k8sClient *k8s.Client
}

// NewConfigHandler 创建配置处理器
func NewConfigHandler(config *models.Config, k8sClient *k8s.Client) *ConfigHandler {
	return &ConfigHandler{
		config:    config,
		k8sClient: k8sClient,
	}
}

// GetConfig 获取系统配置
func (h *ConfigHandler) GetConfig(c *gin.Context) {
	// 获取调度模式，默认 exclusive
	schedulingMode := h.config.GPU.SchedulingMode
	if schedulingMode == "" {
		schedulingMode = "exclusive"
	}

	// 转换存储卷配置为前端展示格式
	storageVolumes := h.getStorageVolumesInfo()

	response := models.ConfigResponse{
		PodLimitPerUser:   h.config.PodLimitPerUser,
		GpuLimitPerUser:   h.config.GpuLimitPerUser,
		GPUTypes:          h.config.GPU.AvailableTypes,
		PresetImages:      h.config.PresetImages,
		UI:                h.config.UI,
		GPUSchedulingMode: schedulingMode,
		MaxPodsPerGPU:     h.config.GPU.MaxPodsPerGPU,
		AllowUserMounts:   h.config.Storage.AllowUserMounts,
		StorageVolumes:    storageVolumes,
		RegistryURL:       h.config.Registry.URL,
	}

	// 如果用户已认证，返回其保存的镜像列表
	if h.k8sClient != nil {
		if username, exists := auth.GetUsername(c); exists && username != "" {
			email, _ := auth.GetEmail(c)
			userIdentifier := k8s.GetUserIdentifier(username, email)
			namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
			ctx := c.Request.Context()

			imageList, err := h.k8sClient.GetUserImages(ctx, namespace)
			if err == nil && imageList != nil && len(imageList.Images) > 0 {
				// 按 savedAt 降序排列（最新在前）
				sort.Slice(imageList.Images, func(i, j int) bool {
					return imageList.Images[i].SavedAt.After(imageList.Images[j].SavedAt)
				})
				response.UserImages = imageList.Images
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// getStorageVolumesInfo 获取存储卷信息（用于前端展示）
func (h *ConfigHandler) getStorageVolumesInfo() []models.StorageVolumeInfo {
	volumes := h.config.Storage.GetEffectiveVolumes()
	result := make([]models.StorageVolumeInfo, 0, len(volumes))

	for _, vol := range volumes {
		info := models.StorageVolumeInfo{
			Name:        vol.Name,
			MountPath:   vol.MountPath,
			Description: vol.Description,
			ReadOnly:    vol.ReadOnly,
			Type:        vol.Type,
			Scope:       vol.Scope,
		}
		// 默认 scope 为 user
		if info.Scope == "" {
			info.Scope = "user"
		}
		result = append(result, info)
	}

	return result
}

