package k8s

import (
	"os"
	"path/filepath"

	"github.com/uc-package/genet/internal/models"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client K8s 客户端
type Client struct {
	clientset *kubernetes.Clientset
	config    *models.Config
}

// NewClient 创建新的 K8s 客户端
func NewClient(config *models.Config) (*Client, error) {
	var restConfig *rest.Config
	var err error

	// 优先使用 KUBECONFIG 环境变量
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		// 尝试使用默认路径
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	// 检查文件是否存在
	if _, err := os.Stat(kubeconfig); err == nil {
		// 使用 kubeconfig 文件
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	} else {
		// 使用 InCluster 配置（在 Pod 内运行时）
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return &Client{
		clientset: clientset,
		config:    config,
	}, nil
}

// GetClientset 获取 Kubernetes clientset
func (c *Client) GetClientset() *kubernetes.Clientset {
	return c.clientset
}

// GetConfig 获取配置
func (c *Client) GetConfig() *models.Config {
	return c.config
}

