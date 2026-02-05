package registry

import (
	"context"
	"fmt"

	"github.com/uc-package/genet/internal/models"
)

// ImageInfo 镜像信息
type ImageInfo struct {
	Name        string   `json:"name"`                  // 镜像名称（不含 registry 前缀）
	Tags        []string `json:"tags,omitempty"`        // 可用 tags
	Description string   `json:"description,omitempty"` // 描述信息（Harbor 支持）
}

// Client Registry 客户端接口
type Client interface {
	// SearchImages 搜索镜像（模糊匹配）
	SearchImages(ctx context.Context, keyword string, limit int) ([]ImageInfo, error)
	// GetImageTags 获取镜像的 tags，platform 为空时返回所有 tags
	GetImageTags(ctx context.Context, imageName string, platform string) ([]string, error)
	// IsConfigured 检查 registry 是否已配置
	IsConfigured() bool
}

// NewClient 根据配置创建 Registry 客户端
func NewClient(config *models.RegistryConfig) (Client, error) {
	if config == nil || config.URL == "" {
		return &noopClient{}, nil
	}

	registryType := config.Type
	if registryType == "" {
		registryType = "docker"
	}

	switch registryType {
	case "harbor":
		return NewHarborClient(config), nil
	case "docker":
		return NewDockerClient(config), nil
	default:
		return nil, fmt.Errorf("unsupported registry type: %s", registryType)
	}
}

// noopClient 空实现，用于未配置 registry 的情况
type noopClient struct{}

func (c *noopClient) SearchImages(ctx context.Context, keyword string, limit int) ([]ImageInfo, error) {
	return []ImageInfo{}, nil
}

func (c *noopClient) GetImageTags(ctx context.Context, imageName string, platform string) ([]string, error) {
	return []string{}, nil
}

func (c *noopClient) IsConfigured() bool {
	return false
}
