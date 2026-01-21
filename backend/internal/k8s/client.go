package k8s

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// k8sLog K8s 模块日志
var k8sLog *zap.Logger

func init() {
	k8sLog = logger.Named("k8s")
}

// Client K8s 客户端
type Client struct {
	clientset *kubernetes.Clientset
	config    *models.Config
	log       *zap.Logger
}

// NewClient 创建新的 K8s 客户端
func NewClient(config *models.Config) (*Client, error) {
	log := logger.Named("k8s")
	var restConfig *rest.Config
	var err error
	var configSource string

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
		log.Info("Using kubeconfig file", zap.String("path", kubeconfig))
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Error("Failed to build config from kubeconfig", zap.Error(err))
			return nil, err
		}
		configSource = "kubeconfig"
	} else {
		// 使用 InCluster 配置（在 Pod 内运行时）
		log.Info("Using in-cluster config")
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			log.Error("Failed to get in-cluster config", zap.Error(err))
			return nil, err
		}
		configSource = "in-cluster"
	}

	// 应用 Kubernetes 配置
	// 1. 禁用代理（解决 Windows 系统代理冲突）
	if config.Kubernetes.DisableProxy {
		log.Debug("Disabling HTTP proxy for K8s client")
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
	log.Debug("K8s client timeout configured", zap.Int("seconds", timeout))

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Error("Failed to create K8s clientset", zap.Error(err))
		return nil, err
	}

	// 测试连接（快速失败）
	log.Debug("Testing K8s connection...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		log.Error("K8s connection test failed", zap.Error(err))
		return nil, fmt.Errorf("kubernetes connection test failed: %w", err)
	}

	log.Info("K8s client initialized successfully",
		zap.String("configSource", configSource),
		zap.Int("timeout", timeout))

	return &Client{
		clientset: clientset,
		config:    config,
		log:       log,
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
