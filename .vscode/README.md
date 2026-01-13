# VSCode 调试配置说明

本目录包含 Genet 项目的 VSCode 调试配置，支持 **Windows** 和 **macOS** 双平台。

## 📋 前置要求

### 通用要求

- [VSCode](https://code.visualstudio.com/) 最新版本
- [Docker Desktop](https://www.docker.com/products/docker-desktop/) 并启用 Kubernetes
- [Go 1.21+](https://golang.org/dl/)
- [Node.js 18+](https://nodejs.org/)
- [Chrome 浏览器](https://www.google.com/chrome/)

### VSCode 扩展

安装推荐的扩展（打开项目时 VSCode 会提示）：
- Go (`golang.go`)
- ESLint (`dbaeumer.vscode-eslint`)
- Prettier (`esbenp.prettier-vscode`)
- Debugger for Chrome (`msjsdiag.debugger-for-chrome`)

## 🔧 配置说明

### kubeconfig

`kubeconfig` 文件用于连接本地 Kubernetes 集群（Docker Desktop）。

**配置方法**：

1. **macOS/Linux**：
   ```bash
   cp ~/.kube/config .vscode/kubeconfig
   ```

2. **Windows (PowerShell)**：
   ```powershell
   Copy-Item $env:USERPROFILE\.kube\config .vscode\kubeconfig
   ```

**注意**：确保 `kubeconfig` 中的 `server` 地址是可访问的（通常是 `https://127.0.0.1:6443`）。

### config.yaml

`config.yaml` 是后端的配置文件，包含 OAuth、存储、GPU 等配置。

根据需要修改：
- `oauth.enabled`: 是否启用 OAuth（本地测试可设为 `false`）
- `storage.storageClass`: 存储类（Docker Desktop 使用 `hostpath`）

## 🚀 调试配置

### 后端调试

| 配置名称 | 说明 |
|---------|------|
| `Backend: API Server` | 调试后端 API 服务 |
| `Backend: Lifecycle Controller` | 调试生命周期控制器 |
| `Backend: Attach to Process` | 附加到已运行的 Go 进程 |
| `Backend Tests` | 运行所有后端测试 |
| `Backend Tests (Current File)` | 运行当前文件的测试 |

### 前端调试

| 配置名称 | 说明 |
|---------|------|
| `Frontend: Chrome Debug` | 使用 Chrome 调试前端 |

### 全栈调试

| 配置名称 | 说明 |
|---------|------|
| `Full Stack Debug` | 同时启动后端 + Chrome 前端调试 |

## 📝 使用步骤

### 1. 准备 kubeconfig

```bash
# macOS/Linux
cp ~/.kube/config .vscode/kubeconfig

# Windows (PowerShell)
Copy-Item $env:USERPROFILE\.kube\config .vscode\kubeconfig
```

### 2. 安装依赖

```bash
# 后端
cd backend && go mod download

# 前端
cd frontend && npm install
```

### 3. 启动 Mock OAuth（可选）

如果 `config.yaml` 中 `oauth.enabled: true`：

```bash
# 使用 VSCode Task
# Ctrl+Shift+P -> "Tasks: Run Task" -> "Mock OAuth: Start"

# 或手动运行
docker run -d --name mock-oauth2-server -p 8888:8080 ghcr.io/navikt/mock-oauth2-server:2.1.0
```

### 4. 开始调试

1. 按 `F5` 或从调试面板选择配置
2. 选择 `Full Stack Debug` 进行全栈调试
3. 等待后端和前端都启动完成
4. Chrome 会自动打开 `http://localhost:3000`

## ⚠️ 平台特定注意事项

### Windows

1. **路径分隔符**：VSCode 会自动处理，无需手动修改
2. **终端**：建议使用 PowerShell
3. **行尾符**：已配置使用 LF 保持跨平台一致

### macOS

1. **Xcode Command Line Tools**：确保已安装
   ```bash
   xcode-select --install
   ```

## 🔍 故障排查

### 后端启动失败

1. 检查 Go 版本：`go version`（需要 1.21+）
2. 检查依赖：`cd backend && go mod download`
3. 检查 kubeconfig：确保 `.vscode/kubeconfig` 文件存在且有效
4. 检查 Kubernetes：`kubectl cluster-info`

### 前端启动失败

1. 检查 Node.js 版本：`node --version`（需要 18+）
2. 检查依赖：`cd frontend && npm install`
3. 检查端口：确保 3000 端口未被占用

### OAuth 登录失败

1. 确保 Mock OAuth 服务已启动
2. 检查 `config.yaml` 中的 OAuth 配置
3. 或将 `oauth.enabled` 设为 `false` 跳过认证

### Kubernetes 连接失败

1. 确保 Docker Desktop 已启动
2. 确保 Kubernetes 已启用（Docker Desktop 设置）
3. 检查 kubeconfig：
   ```bash
   kubectl --kubeconfig=.vscode/kubeconfig cluster-info
   ```

## 📂 文件说明

```
.vscode/
├── config.yaml      # 后端配置文件
├── extensions.json  # 推荐的 VSCode 扩展
├── kubeconfig       # Kubernetes 配置（需自行创建）
├── launch.json      # 调试启动配置
├── settings.json    # VSCode 工作区设置
├── tasks.json       # 任务配置
└── README.md        # 本文件
```

## 🔗 相关文档

- [项目 README](../README.md)
