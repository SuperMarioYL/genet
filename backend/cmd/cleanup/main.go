package main

import (
	"log"
	"os"

	"github.com/uc-package/genet/internal/cleanup"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
)

func main() {
	// 加载配置（优先使用环境变量指定的路径）
	configPath := os.Getenv("GENET_CONFIG")
	if configPath == "" {
		configPath = "/etc/genet/config.yaml"
	}
	config, err := models.LoadConfig(configPath)
	if err != nil {
		log.Printf("Warning: Failed to load config from %s: %v, using defaults", configPath, err)
		config = models.DefaultConfig()
	}

	// 初始化 K8s 客户端
	k8sClient, err := k8s.NewClient(config)
	if err != nil {
		log.Fatalf("Failed to initialize K8s client: %v", err)
	}

	// 创建清理器
	cleaner := cleanup.NewPodCleaner(k8sClient, config)

	log.Println("Genet Pod Cleanup - triggered by CronJob")

	// 执行清理
	if err := cleaner.CleanupAllPods(); err != nil {
		log.Fatalf("Error during cleanup: %v", err)
	}

	log.Println("Cleanup completed successfully")
}
