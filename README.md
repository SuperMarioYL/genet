# Genet - 个人 Pod 申请平台

<div align="center">

基于 Kubernetes 构建的个人 Pod 管理平台，支持 Web UI、SSH/VSCode 连接、自动回收和镜像保存。

[![Go Version](https://img.shields.io/badge/Go-1.21-blue)](https://golang.org)
[![React Version](https://img.shields.io/badge/React-18.2-61dafb)](https://reactjs.org)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

</div>

## ✨ 特性

- 🎨 **统一 Web UI**：无需 CLI，通过浏览器完成所有操作
- 🚀 **会话即 Pod**：状态存储在 Pod annotations，无需数据库
- ⏰ **自动回收**：TTL 机制 + 定时清理，防止资源长期占用
- 📦 **镜像保存**：通过 nerdctl commit 将运行中的 Pod 保存为镜像
- 🎯 **Helm 部署**：一键安装，通过 `--set` 调整参数
- 📊 **配额限制**：限制每用户 Pod 数量和 GPU 总数
- 💻 **SSH 和 VSCode 支持**：hostNetwork 暴露，一键复制连接信息
- 🔐 **OAuth 认证**：内置 OAuth 支持（GitHub、GitLab、OIDC 等）
- 🌐 **代理配置**：支持为 Pod 配置 HTTP/HTTPS 代理

## 🚀 快速开始

### 前置要求

- Kubernetes 集群（1.24+）
- Helm 3.0+
- kubectl 配置好集群访问
- GPU 节点（可选，带 NVIDIA GPU Operator 或 Device Plugin）

### 5 分钟部署

```bash
# 1. 克隆仓库
git clone https://github.com/your-org/genet.git
cd genet

# 2. 构建镜像
make build-images

# 3. 安装系统（禁用认证，适合快速测试）
make install-dev

# 4. 访问系统
kubectl port-forward -n genet-system svc/genet-frontend 8080:80
# 访问 http://localhost:8080
```

### 生产部署

详见 [部署文档](docs/DEPLOYMENT.md)

## 📚 文档

- [部署指南](docs/DEPLOYMENT.md) - 完整的部署和配置说明
- [用户指南](docs/USER_GUIDE.md) - 用户使用手册
- [开发指南](docs/DEVELOPMENT.md) - 开发环境设置和贡献指南

## 🏗️ 架构

```
┌─────────────────────────────────────────────────┐
│              用户浏览器                          │
└────────────────┬────────────────────────────────┘
                 │ HTTPS
     ┌───────────┴───────────┐
     ↓                       ↓
┌──────────┐         ┌──────────────┐
│ React 前端│         │ Go 后端 API   │
└──────────┘         └───────┬───────┘
                             │
                    ┌────────┴────────┐
                    │   client-go     │
                    │   管理用户 Pod   │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │ Kubernetes API  │
                    └─────────────────┘
```

**核心组件**：
- **后端**：Go + client-go（直接操作 K8s API）
- **前端**：React + TypeScript + Ant Design
- **认证**：内置 OAuth 支持（可选）
- **存储**：无数据库设计，状态存于 Pod annotations
- **生命周期**：CronJob 自动回收

## 💡 使用示例

### 创建 Pod

1. 登录系统
2. 点击"创建新 Pod"
3. 选择配置：
   - 镜像：PyTorch 2.0 GPU 或自定义镜像
   - GPU：0-N 个（0 表示纯 CPU Pod）
   - 生命周期：1-24 小时
4. 创建成功后获取 SSH 连接信息

### SSH 连接

```bash
# 直接连接
ssh root@<node-ip> -p <ssh-port>
# 密码: genetpod2024
```

### VSCode Remote 连接

1. 点击 Pod 详情中的 "VSCode" 按钮
2. 或复制 SSH 配置到 `~/.ssh/config`
3. 在 VSCode 中连接：F1 → "Remote-SSH: Connect to Host"

## ⚙️ 配置

### 调整配额

```bash
helm upgrade genet ./helm/genet \
  --set backend.config.podLimitPerUser=10 \
  --set backend.config.gpuLimitPerUser=16
```

### 添加 GPU 类型

编辑 `helm/genet/values.yaml`:

```yaml
backend:
  config:
    gpu:
      availableTypes:
        - name: "NVIDIA H100"
          resourceName: "nvidia.com/gpu"
          nodeSelector:
            gpu-type: "h100"
```

### 配置代理

```yaml
backend:
  config:
    proxy:
      httpProxy: "http://proxy.example.com:8080"
      httpsProxy: "http://proxy.example.com:8080"
      noProxy: "localhost,127.0.0.1,.cluster.local"
```

### 修改自动删除时间

```yaml
backend:
  config:
    lifecycle:
      autoDeleteTime: "22:00"  # 晚上10点
      timezone: "Asia/Shanghai"
```

### 配置额外存储

```yaml
backend:
  config:
    storage:
      extraVolumes:
        - name: "datasets"
          mountPath: "/data/datasets"
          readOnly: true
          pvc: "shared-datasets"
        - name: "models"
          mountPath: "/data/models"
          nfs:
            server: "nfs.example.com"
            path: "/exports/models"
```

## 🛠️ 开发

### 初始化开发环境

```bash
# 安装所有依赖
make init
```

### 后端开发

```bash
cd backend
go mod download
export KUBECONFIG=~/.kube/config
go run cmd/api/main.go --config=../.vscode/config.yaml
```

### 前端开发

```bash
cd frontend
npm install
npm start
```

### 本地调试

使用 VSCode 的 "Full Stack Debug" 配置，可同时启动前后端和 Mock OAuth 服务。

详见 [开发指南](docs/DEVELOPMENT.md)

## 🤝 贡献

欢迎贡献！请查看 [开发指南](docs/DEVELOPMENT.md) 了解如何参与项目。

## 📝 License

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件

## 🙏 致谢

- [Kubernetes](https://kubernetes.io/)
- [Helm](https://helm.sh/)
- [React](https://reactjs.org/)
- [Ant Design](https://ant.design/)
- [nerdctl](https://github.com/containerd/nerdctl)

## 📧 联系方式

- 问题反馈：[GitHub Issues](https://github.com/your-org/genet/issues)
- 邮箱：support@example.com
