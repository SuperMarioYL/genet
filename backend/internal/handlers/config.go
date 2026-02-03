package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/models"
)

// ConfigHandler 配置处理器
type ConfigHandler struct {
	config *models.Config
}

// NewConfigHandler 创建配置处理器
func NewConfigHandler(config *models.Config) *ConfigHandler {
	return &ConfigHandler{
		config: config,
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
		PresetImages:      h.config.GPU.PresetImages,
		UI:                h.config.UI,
		GPUSchedulingMode: schedulingMode,
		MaxPodsPerGPU:     h.config.GPU.MaxPodsPerGPU,
		AllowUserMounts:   h.config.Storage.AllowUserMounts,
		StorageVolumes:    storageVolumes,
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

