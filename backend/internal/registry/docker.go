package registry

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/models"
)

// DockerClient Docker Registry V2 客户端
type DockerClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewDockerClient 创建 Docker Registry 客户端
func NewDockerClient(config *models.RegistryConfig) *DockerClient {
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

	return &DockerClient{
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
func (c *DockerClient) IsConfigured() bool {
	return c.baseURL != ""
}

// catalogResponse Docker Registry _catalog API 响应
type catalogResponse struct {
	Repositories []string `json:"repositories"`
}

// tagsResponse Docker Registry tags/list API 响应
type tagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// SearchImages 搜索镜像（通过获取 catalog 并过滤）
func (c *DockerClient) SearchImages(ctx context.Context, keyword string, limit int) ([]ImageInfo, error) {
	// Docker Registry V2 没有原生搜索 API，需要获取 catalog 后在内存中过滤
	catalogURL := fmt.Sprintf("%s/v2/_catalog", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", catalogURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("catalog request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get catalog failed with status %d: %s", resp.StatusCode, string(body))
	}

	var catalog catalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	// 过滤匹配的镜像
	images := make([]ImageInfo, 0)
	keywordLower := strings.ToLower(keyword)
	for _, repo := range catalog.Repositories {
		if keyword == "" || strings.Contains(strings.ToLower(repo), keywordLower) {
			images = append(images, ImageInfo{
				Name: repo,
			})
			if limit > 0 && len(images) >= limit {
				break
			}
		}
	}

	return images, nil
}

// GetImageTags 获取镜像的 tags（Docker Registry V2 不支持 platform 过滤，忽略 platform 参数）
func (c *DockerClient) GetImageTags(ctx context.Context, imageName string, platform string) ([]string, error) {
	if imageName == "" {
		return []string{}, nil
	}

	tagsURL := fmt.Sprintf("%s/v2/%s/tags/list", c.baseURL, imageName)

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

	var tags tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	return tags.Tags, nil
}
