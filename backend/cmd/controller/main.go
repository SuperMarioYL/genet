package main

import (
	"log"
	"os"

	"github.com/uc-package/genet/internal/controller"
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

	// 创建控制器
	ctrl := controller.NewLifecycleController(k8sClient, config)

	log.Println("Genet lifecycle controller - one-shot mode (for CronJob)")
	log.Printf("Auto-delete time: %s %s", config.Lifecycle.AutoDeleteTime, config.Lifecycle.Timezone)

	// 运行一次协调（CronJob 模式）
	if err := ctrl.ReconcileAll(); err != nil {
		log.Fatalf("Error during reconciliation: %v", err)
	}

	log.Println("Reconciliation completed successfully")
}
