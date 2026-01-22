package main

import (
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/handlers"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"github.com/uc-package/genet/internal/oidc"
	"go.uber.org/zap"
)

func main() {
	// 初始化日志（默认配置，可通过环境变量覆盖）
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "console"
	}
	if err := logger.Init(&logger.Config{
		Level:      logLevel,
		Format:     logFormat,
		OutputPath: "stdout",
	}); err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	log := logger.Named("main")
	log.Info("Genet API server starting...")

	// 加载配置（优先使用环境变量指定的路径）
	configPath := os.Getenv("GENET_CONFIG")
	if configPath == "" {
		configPath = "/etc/genet/config.yaml"
	}
	config, err := models.LoadConfig(configPath)
	if err != nil {
		log.Warn("Failed to load config, using defaults",
			zap.String("path", configPath),
			zap.Error(err))
		config = models.DefaultConfig()
	} else {
		log.Info("Config loaded successfully", zap.String("path", configPath))
	}

	// 初始化认证中间件
	auth.InitAuthMiddleware(config)
	log.Info("Auth middleware initialized")

	// 初始化 OAuth Handler
	oauthHandler := auth.NewOAuthHandler(config)
	if config.OAuth.Enabled {
		if err := oauthHandler.DiscoverOIDC(); err != nil {
			log.Warn("Failed to discover OIDC", zap.Error(err))
		} else {
			log.Info("OIDC discovery successful",
				zap.String("provider", config.OAuth.ProviderURL))
		}
	}

	// 初始化 K8s 客户端
	log.Info("Initializing K8s client...")
	k8sClient, err := k8s.NewClient(config)
	if err != nil {
		log.Fatal("Failed to initialize K8s client", zap.Error(err))
	}
	log.Info("K8s client initialized successfully")

	// 初始化处理器
	podHandler := handlers.NewPodHandler(k8sClient, config)
	configHandler := handlers.NewConfigHandler(config)
	authHandler := handlers.NewAuthHandler(config)
	kubeconfigHandler := handlers.NewKubeconfigHandler(config, k8sClient)
	log.Info("Handlers initialized")

	// 初始化 OIDC Provider（如果启用）
	var oidcProvider *oidc.Provider
	if config.OIDCProvider.Enabled {
		var err error
		oidcProvider, err = oidc.NewProvider(config, k8sClient)
		if err != nil {
			log.Warn("Failed to initialize OIDC Provider", zap.Error(err))
		} else {
			log.Info("OIDC Provider initialized",
				zap.String("issuer", config.OIDCProvider.IssuerURL))
		}
	}

	// 设置 Gin 模式
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建 Gin 引擎（不使用默认中间件）
	r := gin.New()

	// 使用自定义日志中间件
	r.Use(logger.GinRecovery())
	r.Use(logger.GinLogger())

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

	// 注册 OIDC Provider 路由（如果启用）
	if oidcProvider != nil {
		oidcProvider.RegisterRoutes(r)
		// 注册 OIDC 回调路由（处理企业 OAuth 回调）
		r.GET("/oidc/callback", oidcProvider.OAuthCallback)
		log.Info("OIDC Provider routes registered")
	}

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

		// 集群信息（公开）
		api.GET("/cluster/info", kubeconfigHandler.GetClusterInfo)

		// Pod 管理端点（需要认证）
		pods := api.Group("/pods")
		pods.Use(auth.AuthMiddleware(config))
		{
			pods.GET("", podHandler.ListPods)
			pods.POST("", podHandler.CreatePod)
			pods.GET("/:id", podHandler.GetPod)
			pods.DELETE("/:id", podHandler.DeletePod)
			pods.GET("/:id/logs", podHandler.GetPodLogs)
			pods.GET("/:id/events", podHandler.GetPodEvents)
			pods.GET("/:id/describe", podHandler.GetPodDescribe)
			pods.POST("/:id/build", podHandler.BuildImage)
			// 镜像 commit 相关
			pods.POST("/:id/commit", podHandler.CommitImage)
			pods.GET("/:id/commit/status", podHandler.GetCommitStatus)
			pods.GET("/:id/commit/logs", podHandler.GetCommitLogs)
		}

		// Kubeconfig 端点（需要认证）
		api.GET("/kubeconfig", auth.AuthMiddleware(config), kubeconfigHandler.GetKubeconfig)
		api.GET("/kubeconfig/download", auth.AuthMiddleware(config), kubeconfigHandler.DownloadKubeconfig)
	}

	// 启动服务器
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Info("Server configuration",
		zap.String("port", port),
		zap.Bool("oauth_enabled", config.OAuth.Enabled),
		zap.Bool("oidc_provider_enabled", config.OIDCProvider.Enabled))

	log.Info("Starting HTTP server", zap.String("address", ":"+port))
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server", zap.Error(err))
	}
}
