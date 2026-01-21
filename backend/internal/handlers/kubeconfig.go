package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
)

// KubeconfigHandler Kubeconfig 处理器
type KubeconfigHandler struct {
	config    *models.Config
	k8sClient *k8s.Client
}

// NewKubeconfigHandler 创建 Kubeconfig 处理器
func NewKubeconfigHandler(config *models.Config, k8sClient *k8s.Client) *KubeconfigHandler {
	return &KubeconfigHandler{
		config:    config,
		k8sClient: k8sClient,
	}
}

// KubeconfigResponse Kubeconfig 信息响应
type KubeconfigResponse struct {
	// Kubeconfig 内容（YAML 格式）
	Kubeconfig string `json:"kubeconfig"`
	// 用户 Namespace
	Namespace string `json:"namespace"`
	// 集群名称
	ClusterName string `json:"clusterName"`
	// 安装说明
	Instructions KubeconfigInstructions `json:"instructions"`
}

// KubeconfigInstructions 安装说明
type KubeconfigInstructions struct {
	// kubelogin 安装命令
	InstallKubelogin map[string]string `json:"installKubelogin"`
	// 使用说明
	Usage []string `json:"usage"`
}

// GetKubeconfig 获取用户的 kubeconfig
func (h *KubeconfigHandler) GetKubeconfig(c *gin.Context) {
	// 检查是否启用了 OIDC Provider
	if !h.config.OIDCProvider.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "OIDC Provider 未启用",
		})
		return
	}

	// 检查集群配置
	if h.config.Cluster.Server == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "集群配置不完整，请联系管理员配置 cluster.server",
		})
		return
	}

	// 获取当前用户
	username, exists := auth.GetUsername(c)
	if !exists || username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未登录",
		})
		return
	}

	// 生成用户 namespace
	namespace := k8s.GetNamespaceForUser(username)

	// 生成 kubeconfig
	kubeconfig := h.generateKubeconfig(username, namespace)

	// 返回响应
	response := KubeconfigResponse{
		Kubeconfig:  kubeconfig,
		Namespace:   namespace,
		ClusterName: h.config.Cluster.Name,
		Instructions: KubeconfigInstructions{
			InstallKubelogin: map[string]string{
				"macOS":   "brew install int128/kubelogin/kubelogin",
				"Linux":   "curl -LO https://github.com/int128/kubelogin/releases/latest/download/kubelogin_linux_amd64.zip && unzip kubelogin_linux_amd64.zip && sudo mv kubelogin /usr/local/bin/kubectl-oidc_login",
				"Windows": "choco install kubelogin",
			},
			Usage: []string{
				"1. 安装 kubelogin（见上方安装命令）",
				"2. 将 kubeconfig 保存到 ~/.kube/config 或使用 KUBECONFIG 环境变量",
				"3. 运行 kubectl get pods，首次会打开浏览器进行登录",
				"4. 登录成功后，Token 会被缓存，后续命令无需重复登录",
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// DownloadKubeconfig 下载 kubeconfig 文件
func (h *KubeconfigHandler) DownloadKubeconfig(c *gin.Context) {
	// 检查是否启用了 OIDC Provider
	if !h.config.OIDCProvider.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "OIDC Provider 未启用",
		})
		return
	}

	// 检查集群配置
	if h.config.Cluster.Server == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "集群配置不完整",
		})
		return
	}

	// 获取当前用户
	username, exists := auth.GetUsername(c)
	if !exists || username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未登录",
		})
		return
	}

	// 生成用户 namespace
	namespace := k8s.GetNamespaceForUser(username)

	// 生成 kubeconfig
	kubeconfig := h.generateKubeconfig(username, namespace)

	// 设置下载头
	filename := fmt.Sprintf("kubeconfig-%s.yaml", username)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/x-yaml")
	c.String(http.StatusOK, kubeconfig)
}

// generateKubeconfig 生成 kubeconfig 内容
func (h *KubeconfigHandler) generateKubeconfig(username, namespace string) string {
	clusterName := h.config.Cluster.Name
	if clusterName == "" {
		clusterName = "genet-cluster"
	}

	issuerURL := strings.TrimSuffix(h.config.OIDCProvider.IssuerURL, "/")
	clientID := h.config.OIDCProvider.KubernetesClientID
	clientSecret := h.config.OIDCProvider.KubernetesClientSecret

	// 构建 kubeconfig YAML
	var sb strings.Builder
	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: Config\n")
	sb.WriteString("clusters:\n")
	sb.WriteString(fmt.Sprintf("- name: %s\n", clusterName))
	sb.WriteString("  cluster:\n")
	sb.WriteString(fmt.Sprintf("    server: %s\n", h.config.Cluster.Server))
	if h.config.Cluster.CAData != "" {
		sb.WriteString(fmt.Sprintf("    certificate-authority-data: %s\n", h.config.Cluster.CAData))
	}
	sb.WriteString("contexts:\n")
	sb.WriteString(fmt.Sprintf("- name: %s\n", clusterName))
	sb.WriteString("  context:\n")
	sb.WriteString(fmt.Sprintf("    cluster: %s\n", clusterName))
	sb.WriteString("    user: oidc\n")
	sb.WriteString(fmt.Sprintf("    namespace: %s\n", namespace))
	sb.WriteString(fmt.Sprintf("current-context: %s\n", clusterName))
	sb.WriteString("users:\n")
	sb.WriteString("- name: oidc\n")
	sb.WriteString("  user:\n")
	sb.WriteString("    exec:\n")
	sb.WriteString("      apiVersion: client.authentication.k8s.io/v1beta1\n")
	sb.WriteString("      command: kubectl\n")
	sb.WriteString("      args:\n")
	sb.WriteString("      - oidc-login\n")
	sb.WriteString("      - get-token\n")
	sb.WriteString(fmt.Sprintf("      - --oidc-issuer-url=%s\n", issuerURL))
	sb.WriteString(fmt.Sprintf("      - --oidc-client-id=%s\n", clientID))
	if clientSecret != "" {
		sb.WriteString(fmt.Sprintf("      - --oidc-client-secret=%s\n", clientSecret))
	}

	return sb.String()
}

// GetClusterInfo 获取集群信息（公开接口，用于前端显示）
type ClusterInfoResponse struct {
	OIDCEnabled bool   `json:"oidcEnabled"`
	ClusterName string `json:"clusterName,omitempty"`
	IssuerURL   string `json:"issuerURL,omitempty"`
}

// GetClusterInfo 获取集群信息
func (h *KubeconfigHandler) GetClusterInfo(c *gin.Context) {
	response := ClusterInfoResponse{
		OIDCEnabled: h.config.OIDCProvider.Enabled,
	}

	if h.config.OIDCProvider.Enabled {
		response.ClusterName = h.config.Cluster.Name
		response.IssuerURL = h.config.OIDCProvider.IssuerURL
	}

	c.JSON(http.StatusOK, response)
}
