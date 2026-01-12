.PHONY: help build-backend build-frontend build-images push-images install install-dev uninstall test lint clean
.PHONY: mock-oauth-start mock-oauth-stop mock-oauth-logs dev-auth-status

# 变量定义
REGISTRY ?= your-registry.io
IMAGE_TAG ?= latest
NAMESPACE ?= genet-system

BACKEND_IMAGE = $(REGISTRY)/genet-backend:$(IMAGE_TAG)
FRONTEND_IMAGE = $(REGISTRY)/genet-frontend:$(IMAGE_TAG)

help: ## 显示帮助信息
	@echo "可用的 Make 命令:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# 构建相关
build-backend: ## 构建后端二进制
	@echo "==> 构建后端..."
	cd backend && go build -o bin/api-server ./cmd/api
	cd backend && go build -o bin/controller ./cmd/controller

build-frontend: ## 构建前端
	@echo "==> 构建前端..."
	cd frontend && npm install && npm run build

build-backend-image: ## 构建后端 Docker 镜像
	@echo "==> 构建后端镜像..."
	docker build -t $(BACKEND_IMAGE) -f backend/Dockerfile backend/

build-frontend-image: ## 构建前端 Docker 镜像
	@echo "==> 构建前端镜像..."
	docker build -t $(FRONTEND_IMAGE) -f frontend/Dockerfile frontend/

build-images: build-backend-image build-frontend-image ## 构建所有 Docker 镜像

# 推送镜像
push-images: ## 推送 Docker 镜像到仓库
	@echo "==> 推送镜像..."
	docker push $(BACKEND_IMAGE)
	docker push $(FRONTEND_IMAGE)

# 部署相关
install: ## 安装 Genet 系统（生产环境）
	@echo "==> 安装 Genet 系统..."
	helm upgrade --install genet ./helm/genet \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--set backend.image=$(BACKEND_IMAGE) \
		--set frontend.image=$(FRONTEND_IMAGE) \
		--wait

install-dev: ## 安装 Genet 系统（开发环境）
	@echo "==> 安装 Genet 系统 (开发环境)..."
	helm upgrade --install genet ./helm/genet \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--set backend.image=$(BACKEND_IMAGE) \
		--set frontend.image=$(FRONTEND_IMAGE) \
		--set backend.imagePullPolicy=Always \
		--set frontend.imagePullPolicy=Always \
		--set ingress.enabled=false \
		--wait

uninstall: ## 卸载 Genet 系统
	@echo "==> 卸载 Genet 系统..."
	helm uninstall genet -n $(NAMESPACE) || true
	kubectl delete namespace $(NAMESPACE) || true

# 测试相关
test: ## 运行测试
	@echo "==> 运行后端测试..."
	cd backend && go test -v ./...
	@echo "==> 运行前端测试..."
	cd frontend && npm test -- --watchAll=false

test-backend: ## 运行后端测试
	@echo "==> 运行后端测试..."
	cd backend && go test -v ./...

test-frontend: ## 运行前端测试
	@echo "==> 运行前端测试..."
	cd frontend && npm test -- --watchAll=false

# 代码质量
lint: ## 运行 Linter
	@echo "==> 运行 Go Linter..."
	cd backend && golangci-lint run
	@echo "==> 运行 ESLint..."
	cd frontend && npm run lint

fmt: ## 格式化代码
	@echo "==> 格式化 Go 代码..."
	cd backend && go fmt ./...
	@echo "==> 格式化前端代码..."
	cd frontend && npm run format

# 开发相关
dev-backend: ## 启动后端开发服务器
	@echo "==> 启动后端开发服务器..."
	cd backend && go run ./cmd/api/main.go

dev-frontend: ## 启动前端开发服务器
	@echo "==> 启动前端开发服务器..."
	cd frontend && npm start

dev-controller: ## 启动控制器开发服务器
	@echo "==> 启动控制器开发服务器..."
	cd backend && go run ./cmd/controller/main.go

# Kubernetes 相关
k8s-logs-backend: ## 查看后端日志
	kubectl logs -n $(NAMESPACE) -l app=genet-backend -f

k8s-logs-frontend: ## 查看前端日志
	kubectl logs -n $(NAMESPACE) -l app=genet-frontend -f

k8s-logs-controller: ## 查看控制器日志
	kubectl logs -n $(NAMESPACE) -l app=genet-controller -f

k8s-port-forward: ## 端口转发
	kubectl port-forward -n $(NAMESPACE) svc/genet-frontend 8080:80

k8s-status: ## 查看系统状态
	@echo "==> Genet 系统状态:"
	kubectl get all -n $(NAMESPACE)

# 清理
clean: ## 清理构建产物
	@echo "==> 清理构建产物..."
	rm -rf backend/bin
	rm -rf frontend/build

clean-all: clean ## 清理所有（包括依赖）
	@echo "==> 清理所有..."
	rm -rf backend/vendor
	rm -rf frontend/node_modules

# 初始化
init: ## 初始化开发环境
	@echo "==> 初始化开发环境..."
	@echo "-> 安装后端依赖..."
	cd backend && go mod download
	@echo "-> 安装前端依赖..."
	cd frontend && npm install
	@echo "==> 初始化完成!"

# 版本管理
version: ## 显示版本信息
	@echo "Genet Version Information:"
	@echo "  Backend:  $(shell cd backend && go list -m)"
	@echo "  Frontend: $(shell cd frontend && node -p "require('./package.json').version")"
	@echo "  Helm:     $(shell grep 'version:' helm/genet/Chart.yaml | awk '{print $$2}')"

# 一键部署
deploy-all: build-images push-images install ## 一键构建、推送和部署

# 快速迭代（本地开发）
dev: ## 启动完整开发环境
	@echo "==> 启动开发环境..."
	@echo "请在不同的终端窗口运行以下命令:"
	@echo "  1. make dev-backend"
	@echo "  2. make dev-frontend"
	@echo "  3. make dev-controller"

# ============================================
# OAuth2 本地测试
# ============================================
MOCK_OAUTH_CONTAINER = genet-mock-oauth

mock-oauth-start: ## 启动 Mock OAuth2 Server
	@echo "==> 启动 Mock OAuth2 Server..."
	@docker rm -f $(MOCK_OAUTH_CONTAINER) 2>/dev/null || true
	docker run -d \
		--name $(MOCK_OAUTH_CONTAINER) \
		-p 8888:8080 \
		-e JSON_CONFIG='{"interactiveLogin":true}' \
		ghcr.io/navikt/mock-oauth2-server:2.1.0
	@echo ""
	@echo "============================================"
	@echo "Mock OAuth2 Server 已启动!"
	@echo "============================================"
	@echo ""
	@echo "服务地址: http://localhost:8888"
	@echo "OIDC Discovery: http://localhost:8888/default/.well-known/openid-configuration"
	@echo ""
	@echo "使用方式:"
	@echo "  1. 启动后端: make dev-backend"
	@echo "  2. 启动前端: make dev-frontend"
	@echo "  3. 访问: http://localhost:3000"
	@echo ""
	@echo "Mock 登录时可以输入任意用户名/邮箱"
	@echo ""

mock-oauth-stop: ## 停止 Mock OAuth2 Server
	@echo "==> 停止 Mock OAuth2 Server..."
	@docker rm -f $(MOCK_OAUTH_CONTAINER) 2>/dev/null || true
	@echo "Mock OAuth2 Server 已停止"

mock-oauth-logs: ## 查看 Mock OAuth2 Server 日志
	docker logs -f $(MOCK_OAUTH_CONTAINER)

dev-auth-status: ## 查看 Mock OAuth2 Server 状态
	@echo "==> Mock OAuth2 Server 状态:"
	@docker ps --filter "name=$(MOCK_OAUTH_CONTAINER)" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

