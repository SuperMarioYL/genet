package main

import (
	"log"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/handlers"
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

	// 初始化认证中间件
	auth.InitAuthMiddleware(config)

	// 初始化 OAuth Handler
	oauthHandler := auth.NewOAuthHandler(config)
	if config.OAuth.Enabled {
		if err := oauthHandler.DiscoverOIDC(); err != nil {
			log.Printf("Warning: Failed to discover OIDC: %v", err)
		} else {
			log.Printf("OIDC discovery successful for provider: %s", config.OAuth.ProviderURL)
		}
	}

	// 初始化 K8s 客户端
	k8sClient, err := k8s.NewClient(config)
	if err != nil {
		log.Fatalf("Failed to initialize K8s client: %v", err)
	}

	// 初始化处理器
	podHandler := handlers.NewPodHandler(k8sClient, config)
	configHandler := handlers.NewConfigHandler(config)
	authHandler := handlers.NewAuthHandler(config)

	// 设置 Gin 路由
	r := gin.Default()

	// CORS 配置
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Auth-Request-User", "X-Auth-Request-Email"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// API 路由
	api := r.Group("/api")
	{
		// 公开端点（不需要认证）
		api.GET("/config", configHandler.GetConfig)
		api.GET("/auth/status", auth.AuthMiddleware(config), authHandler.GetAuthStatus)

		// OAuth 认证端点（公开）
		api.GET("/auth/login", oauthHandler.Login)
		api.GET("/auth/callback", oauthHandler.Callback)
		api.GET("/auth/logout", oauthHandler.Logout)

		// Pod 管理端点（需要认证）
		pods := api.Group("/pods")
		pods.Use(auth.AuthMiddleware(config))
		{
			pods.GET("", podHandler.ListPods)
			pods.POST("", podHandler.CreatePod)
			pods.GET("/:id", podHandler.GetPod)
			pods.POST("/:id/extend", podHandler.ExtendPod)
			pods.DELETE("/:id", podHandler.DeletePod)
			pods.GET("/:id/logs", podHandler.GetPodLogs)
			pods.GET("/:id/events", podHandler.GetPodEvents)
			pods.GET("/:id/describe", podHandler.GetPodDescribe)
			pods.POST("/:id/build", podHandler.BuildImage)
			// 镜像 commit 相关
			pods.POST("/:id/commit", podHandler.CommitImage)
			pods.GET("/:id/commit/status", podHandler.GetCommitStatus)
			pods.GET("/:id/commit/logs", podHandler.GetCommitLogs)
			// Xshell 会话文件下载 (id 格式: namespace/name)
			pods.GET("/:id/xshell", podHandler.DownloadXshellFile)
		}
	}

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Genet API server on port %s", port)
	log.Printf("OAuth enabled: %v", config.OAuth.Enabled)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
