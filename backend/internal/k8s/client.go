package k8s

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/uc-package/genet/internal/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// 应用 Kubernetes 配置
	// 1. 禁用代理（解决 Windows 系统代理冲突）
	if config.Kubernetes.DisableProxy {
		restConfig.Proxy = func(req *http.Request) (*url.URL, error) {
			return nil, nil
		}
	}

	// 2. 设置超时时间
	timeout := config.Kubernetes.Timeout
	if timeout <= 0 {
		timeout = 30 // 默认 30 秒
	}
	restConfig.Timeout = time.Duration(timeout) * time.Second

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	// 测试连接（快速失败）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return nil, fmt.Errorf("kubernetes connection test failed: %w", err)
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

