package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
)

// KubeconfigHandler Kubeconfig 处理器
type KubeconfigHandler struct {
	config    *models.Config
	k8sClient *k8s.Client
	log       *zap.Logger
}

// NewKubeconfigHandler 创建 Kubeconfig 处理器
func NewKubeconfigHandler(config *models.Config, k8sClient *k8s.Client) *KubeconfigHandler {
	return &KubeconfigHandler{
		config:    config,
		k8sClient: k8sClient,
		log:       logger.Named("kubeconfig"),
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
	// 认证模式
	Mode string `json:"mode"`
	// 安装说明
	Instructions KubeconfigInstructions `json:"instructions"`
}

// KubeconfigInstructions 安装说明
type KubeconfigInstructions struct {
	// kubelogin 安装命令（仅 OIDC 模式）
	InstallKubelogin map[string]string `json:"installKubelogin,omitempty"`
	// 使用说明
	Usage []string `json:"usage"`
}

// GetKubeconfig 获取用户的 kubeconfig
func (h *KubeconfigHandler) GetKubeconfig(c *gin.Context) {
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

	// 根据模式生成 kubeconfig
	mode := h.config.Kubeconfig.Mode
	if mode == "" {
		mode = "cert" // 默认证书模式
	}

	h.log.Info("Generating kubeconfig",
		zap.String("username", username),
		zap.String("mode", mode))

	var kubeconfig string
	var instructions KubeconfigInstructions
	var err error

	if mode == "oidc" {
		// OIDC 模式
		if !h.config.OIDCProvider.Enabled {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "OIDC Provider 未启用，无法使用 OIDC 模式",
			})
			return
		}
		kubeconfig = h.generateOIDCKubeconfig(username, namespace)
		instructions = KubeconfigInstructions{
			InstallKubelogin: map[string]string{
				"macOS":   "brew install int128/kubelogin/kubelogin",
				"Linux":   "curl -LO https://github.com/int128/kubelogin/releases/latest/download/kubelogin_linux_amd64.zip && unzip kubelogin_linux_amd64.zip && sudo mv kubelogin /usr/local/bin/kubectl-oidc_login",
				"Windows": "choco install kubelogin",
			},
			Usage: []string{
				"安装 kubelogin（见上方安装命令）",
				"将 kubeconfig 保存到 ~/.kube/config 或使用 KUBECONFIG 环境变量",
				"运行 kubectl get pods，首次会打开浏览器进行登录",
				"登录成功后，Token 会被缓存，后续命令无需重复登录",
			},
		}
	} else {
		// 证书模式
		kubeconfig, err = h.generateCertKubeconfig(c.Request.Context(), username, namespace)
		if err != nil {
			h.log.Error("Failed to generate cert kubeconfig",
				zap.String("username", username),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("生成证书失败: %v", err),
			})
			return
		}
		instructions = KubeconfigInstructions{
			Usage: []string{
				"将 kubeconfig 保存到 ~/.kube/config 或使用 KUBECONFIG 环境变量",
				"直接运行 kubectl get pods 即可",
				fmt.Sprintf("证书有效期: %d 小时（约 %d 天）", h.config.Kubeconfig.CertValidityHours, h.config.Kubeconfig.CertValidityHours/24),
				"证书过期后请重新下载 kubeconfig",
			},
		}
	}

	// 返回响应
	response := KubeconfigResponse{
		Kubeconfig:   kubeconfig,
		Namespace:    namespace,
		ClusterName:  h.config.Cluster.Name,
		Mode:         mode,
		Instructions: instructions,
	}

	c.JSON(http.StatusOK, response)
}

// DownloadKubeconfig 下载 kubeconfig 文件
func (h *KubeconfigHandler) DownloadKubeconfig(c *gin.Context) {
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

	// 根据模式生成 kubeconfig
	mode := h.config.Kubeconfig.Mode
	if mode == "" {
		mode = "cert"
	}

	h.log.Info("Downloading kubeconfig",
		zap.String("username", username),
		zap.String("mode", mode))

	var kubeconfig string
	var err error

	if mode == "oidc" {
		if !h.config.OIDCProvider.Enabled {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "OIDC Provider 未启用",
			})
			return
		}
		kubeconfig = h.generateOIDCKubeconfig(username, namespace)
	} else {
		kubeconfig, err = h.generateCertKubeconfig(c.Request.Context(), username, namespace)
		if err != nil {
			h.log.Error("Failed to generate cert kubeconfig for download",
				zap.String("username", username),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("生成证书失败: %v", err),
			})
			return
		}
	}

	// 设置下载头（文件名为 config，与 kubectl 默认配置文件名一致）
	c.Header("Content-Disposition", "attachment; filename=config")
	c.Header("Content-Type", "application/x-yaml")
	c.String(http.StatusOK, kubeconfig)
}

// generateOIDCKubeconfig 生成 OIDC 模式的 kubeconfig
func (h *KubeconfigHandler) generateOIDCKubeconfig(username, namespace string) string {
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

// generateCertKubeconfig 生成证书模式的 kubeconfig
func (h *KubeconfigHandler) generateCertKubeconfig(ctx context.Context, username, namespace string) (string, error) {
	// 生成用户证书
	validityHours := h.config.Kubeconfig.CertValidityHours
	if validityHours <= 0 {
		validityHours = 8760 // 默认 1 年
	}

	cert, err := h.k8sClient.GenerateUserCertificate(ctx, username, validityHours)
	if err != nil {
		return "", fmt.Errorf("failed to generate certificate: %w", err)
	}

	clusterName := h.config.Cluster.Name
	if clusterName == "" {
		clusterName = "genet-cluster"
	}

	// Base64 编码证书和私钥
	certBase64 := base64.StdEncoding.EncodeToString([]byte(cert.CertificatePEM))
	keyBase64 := base64.StdEncoding.EncodeToString([]byte(cert.PrivateKeyPEM))

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
	sb.WriteString("- name: default\n")
	sb.WriteString("  context:\n")
	sb.WriteString(fmt.Sprintf("    cluster: %s\n", clusterName))
	sb.WriteString(fmt.Sprintf("    user: %s\n", username))
	sb.WriteString(fmt.Sprintf("    namespace: %s\n", namespace))
	sb.WriteString("current-context: default\n")
	sb.WriteString("users:\n")
	sb.WriteString(fmt.Sprintf("- name: %s\n", username))
	sb.WriteString("  user:\n")
	sb.WriteString(fmt.Sprintf("    client-certificate-data: %s\n", certBase64))
	sb.WriteString(fmt.Sprintf("    client-key-data: %s\n", keyBase64))

	h.log.Info("Generated cert kubeconfig",
		zap.String("username", username),
		zap.String("namespace", namespace),
		zap.Time("expiresAt", cert.ExpiresAt))

	return sb.String(), nil
}

// GetClusterInfo 获取集群信息（公开接口，用于前端显示）
type ClusterInfoResponse struct {
	OIDCEnabled      bool   `json:"oidcEnabled"`
	KubeconfigMode   string `json:"kubeconfigMode"`
	ClusterName      string `json:"clusterName,omitempty"`
	IssuerURL        string `json:"issuerURL,omitempty"`
	CertValidityDays int    `json:"certValidityDays,omitempty"`
}

// GetClusterInfo 获取集群信息
func (h *KubeconfigHandler) GetClusterInfo(c *gin.Context) {
	mode := h.config.Kubeconfig.Mode
	if mode == "" {
		mode = "cert"
	}

	response := ClusterInfoResponse{
		OIDCEnabled:    h.config.OIDCProvider.Enabled,
		KubeconfigMode: mode,
		ClusterName:    h.config.Cluster.Name,
	}

	if h.config.OIDCProvider.Enabled {
		response.IssuerURL = h.config.OIDCProvider.IssuerURL
	}

	if mode == "cert" {
		validityHours := h.config.Kubeconfig.CertValidityHours
		if validityHours <= 0 {
			validityHours = 8760
		}
		response.CertValidityDays = validityHours / 24
	}

	c.JSON(http.StatusOK, response)
}
