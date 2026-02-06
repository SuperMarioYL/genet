package registry

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/models"
)

// HarborClient Harbor Registry 客户端
type HarborClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewHarborClient 创建 Harbor 客户端
func NewHarborClient(config *models.RegistryConfig) *HarborClient {
	baseURL := strings.TrimSuffix(config.URL, "/")

	// 确保 URL 包含协议
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		if config.Insecure {
			baseURL = "http://" + baseURL
		} else {
			baseURL = "https://" + baseURL
		}
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.Insecure,
		},
	}

	return &HarborClient{
		baseURL:  baseURL,
		username: config.Username,
		password: config.Password,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// IsConfigured 检查是否已配置
func (c *HarborClient) IsConfigured() bool {
	return c.baseURL != ""
}

// harborSearchResult Harbor 搜索 API 响应
type harborSearchResult struct {
	Repository []harborRepository `json:"repository"`
}

type harborRepository struct {
	RepositoryName string `json:"repository_name"`
	ProjectName    string `json:"project_name"`
	ProjectPublic  bool   `json:"project_public"`
	Description    string `json:"description"`
}

// harborTagsResult Harbor tags API 响应
type harborTagsResult struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// SearchImages 搜索镜像
func (c *HarborClient) SearchImages(ctx context.Context, keyword string, limit int) ([]ImageInfo, error) {
	if keyword == "" {
		return []ImageInfo{}, nil
	}

	// Harbor v2.0 搜索 API
	searchURL := fmt.Sprintf("%s/api/v2.0/search?q=%s", c.baseURL, url.QueryEscape(keyword))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result harborSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	images := make([]ImageInfo, 0, len(result.Repository))
	for i, repo := range result.Repository {
		if limit > 0 && i >= limit {
			break
		}
		images = append(images, ImageInfo{
			Name:        repo.RepositoryName,
			Description: repo.Description,
		})
	}

	return images, nil
}

// harborArtifact Harbor artifact 结构（用于 platform 过滤）
type harborArtifact struct {
	Tags []struct {
		Name string `json:"name"`
	} `json:"tags"`
	// 多架构镜像：references 包含各 platform 的子 manifest
	References []struct {
		Platform struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		} `json:"platform"`
	} `json:"references"`
	// 单架构镜像：extra_attrs 中包含架构信息
	ExtraAttrs map[string]interface{} `json:"extra_attrs"`
}

// matchesPlatform 检查 artifact 是否匹配指定 platform
func (a *harborArtifact) matchesPlatform(platform string) bool {
	if platform == "" {
		return true
	}

	// 多架构镜像：检查 references 中是否有匹配的 platform
	if len(a.References) > 0 {
		for _, ref := range a.References {
			if ref.Platform.Architecture == platform {
				return true
			}
		}
		return false
	}

	// 单架构镜像：检查 extra_attrs.architecture
	if arch, ok := a.ExtraAttrs["architecture"]; ok {
		if archStr, ok := arch.(string); ok {
			return archStr == platform
		}
	}

	// 无法判断架构，保留（兼容处理）
	return true
}

// GetImageTags 获取镜像的 tags，支持按 platform 过滤
func (c *HarborClient) GetImageTags(ctx context.Context, imageName string, platform string) ([]string, error) {
	if imageName == "" {
		return []string{}, nil
	}

	// 解析 project/repository 格式
	parts := strings.SplitN(imageName, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid image name format, expected: project/repository")
	}

	project := parts[0]
	repository := url.PathEscape(parts[1])

	// Harbor v2.0 artifacts API
	tagsURL := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts?page_size=100",
		c.baseURL, project, repository)

	req, err := http.NewRequestWithContext(ctx, "GET", tagsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tags request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get tags failed with status %d: %s", resp.StatusCode, string(body))
	}

	var artifacts []harborArtifact
	if err := json.NewDecoder(resp.Body).Decode(&artifacts); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	tags := make([]string, 0)
	for _, artifact := range artifacts {
		if !artifact.matchesPlatform(platform) {
			continue
		}
		for _, tag := range artifact.Tags {
			tags = append(tags, tag.Name)
		}
	}

	return tags, nil
}
