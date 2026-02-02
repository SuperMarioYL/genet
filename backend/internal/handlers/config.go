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

	response := models.ConfigResponse{
		PodLimitPerUser:   h.config.PodLimitPerUser,
		GpuLimitPerUser:   h.config.GpuLimitPerUser,
		GPUTypes:          h.config.GPU.AvailableTypes,
		PresetImages:      h.config.GPU.PresetImages,
		UI:                h.config.UI,
		GPUSchedulingMode: schedulingMode,
		MaxPodsPerGPU:     h.config.GPU.MaxPodsPerGPU,
	}

	c.JSON(http.StatusOK, response)
}

