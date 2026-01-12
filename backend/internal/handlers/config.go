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
	response := models.ConfigResponse{
		PodLimitPerUser: h.config.PodLimitPerUser,
		GpuLimitPerUser: h.config.GpuLimitPerUser,
		GPUTypes:        h.config.GPU.AvailableTypes,
		PresetImages:    h.config.GPU.PresetImages,
		UI:              h.config.UI,
	}

	c.JSON(http.StatusOK, response)
}

