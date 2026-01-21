# K8s OIDC 配置指南

本文档说明如何配置 Kubernetes 集群使用 Genet 作为 OIDC Provider，实现用户通过 OIDC 登录后直接使用 kubectl 访问集群。

## 架构概述

```
┌─────────────────────────────────────────────────────────────────┐
│                         用户                                     │
└───────────────────────────┬─────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
┌───────────────┐   ┌───────────────┐   ┌───────────────┐
│  Genet Web UI │   │    kubectl    │   │  VSCode SSH   │
└───────┬───────┘   └───────┬───────┘   └───────┬───────┘
        │                   │                   │
        │ OIDC Login        │ OIDC Token        │ SSH
        ▼                   ▼                   ▼
┌───────────────────────────────────────────────────────────────┐
│                    Genet (OIDC Provider)                       │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ /.well-known/openid-configuration  (Discovery)          │  │
│  │ /oidc/authorize                    (授权端点)            │  │
│  │ /oidc/token                        (Token 端点)          │  │
│  │ /oidc/userinfo                     (用户信息端点)         │  │
│  │ /oidc/jwks                         (公钥端点)            │  │
│  └─────────────────────────────────────────────────────────┘  │
└───────────────────────────┬───────────────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────────────┐
│                    企业内部 OAuth                              │
└───────────────────────────────────────────────────────────────┘
```

## 前置条件

1. Genet 已部署并可通过 HTTPS 访问（如 `https://genet.example.com`）
2. 企业内部 OAuth 系统已配置
3. 有 Kubernetes 集群管理员权限
4. （可选）已安装 cert-manager 用于自动管理 TLS 证书

## 配置步骤

### 1. 配置 Genet

在 `values.yaml` 中启用 OIDC Provider：

```yaml
backend:
  config:
    # 企业 OAuth 配置（上游认证）
    oauth:
      enabled: true
      mode: "oauth"  # 使用纯 OAuth 模式
      authorizationEndpoint: "https://your-oauth.com/oauth/authorize"
      tokenEndpoint: "https://your-oauth.com/oauth/token"
      userinfoEndpoint: "https://your-oauth.com/api/userinfo"
      userinfoMethod: "post"  # 如果 userinfo 需要 POST 请求
      tokenUsernameClaim: "username"
      tokenEmailClaim: "email"
      clientID: "genet"
      clientSecret: "your-client-secret"
      redirectURL: "https://genet.example.com/api/auth/callback"
      frontendURL: "https://genet.example.com"
      scopes:
        - "openid"
        - "profile"
        - "email"
      jwtSecret: "your-strong-jwt-secret-at-least-32-chars"
      cookieSecure: true

    # OIDC Provider 配置（Genet 作为 OIDC Provider）
    oidcProvider:
      enabled: true
      issuerURL: "https://genet.example.com"  # 必须是外部可访问的 HTTPS 地址
      kubernetesClientID: "kubernetes"
      kubernetesClientSecret: "kubernetes-secret"
      # RSA 密钥（可选，留空则自动生成）
      # 生产环境建议配置固定密钥，否则重启后已签发的 Token 会失效
      rsaPrivateKey: ""
      rsaPublicKey: ""

    # K8s 集群配置（用于生成 kubeconfig）
    cluster:
      name: "my-cluster"
      server: "https://k8s-api.example.com:6443"
      caData: "<base64-encoded-ca-cert>"  # 集群 CA 证书

    # 用户 RBAC 配置
    userRBAC:
      enabled: true
      autoCreate: true  # 登录时自动创建用户 Namespace 和 RBAC

# Ingress 配置（启用 HTTPS）
ingress:
  enabled: true
  className: "nginx"
  host: "genet.example.com"
  tls:
    enabled: true
    secretName: "genet-tls"  # 如果使用 cert-manager 可留空
  # cert-manager 自动申请证书
  certManager:
    enabled: true
    clusterIssuer: "letsencrypt-prod"

# cert-manager 配置（可选，创建 ClusterIssuer）
certManager:
  createClusterIssuer: true
  clusterIssuerName: "letsencrypt-prod"
  email: "admin@example.com"  # 接收证书过期通知
```

### 1.1 配置 TLS 证书（三种方式）

#### 方式一：使用 cert-manager 自签名证书（内网推荐）

```bash
# 1. 安装 cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# 2. 等待 cert-manager 就绪
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=300s

# 3. 部署 Genet（自动创建自签名 CA 和证书）
helm upgrade --install genet ./helm/genet \
  --namespace genet-system --create-namespace \
  --set ingress.host=genet.internal.com \
  --set ingress.tls.enabled=true \
  --set ingress.certManager.enabled=true \
  --set certManager.createClusterIssuer=true \
  --set certManager.type=selfsigned

# 4. 获取 CA 证书（用于配置 K8s API Server 信任）
kubectl get secret genet-ca-secret -n genet-system -o jsonpath='{.data.ca\.crt}' | base64 -d > genet-ca.crt

# 5. 配置 K8s API Server 信任该 CA
# --oidc-ca-file=/etc/kubernetes/pki/genet-ca.crt
```

#### 方式二：使用 cert-manager + Let's Encrypt（公网环境）

```bash
# 1. 安装 cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# 2. 等待 cert-manager 就绪
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=300s

# 3. 部署 Genet（会自动创建 ClusterIssuer 和申请证书）
helm upgrade --install genet ./helm/genet \
  --set ingress.tls.enabled=true \
  --set ingress.certManager.enabled=true \
  --set ingress.certManager.clusterIssuer=letsencrypt-prod \
  --set certManager.createClusterIssuer=true \
  --set certManager.type=acme \
  --set certManager.email=admin@example.com
```

#### 方式二：使用已有证书

```bash
# 1. 创建 TLS Secret
kubectl create secret tls genet-tls \
  --cert=path/to/tls.crt \
  --key=path/to/tls.key \
  -n genet-system

# 2. 部署 Genet
helm upgrade --install genet ./helm/genet \
  --set ingress.tls.enabled=true \
  --set ingress.tls.secretName=genet-tls
```

#### 方式三：使用自签名证书（开发/测试）

```bash
# 1. 生成自签名证书
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout genet.key -out genet.crt \
  -subj "/CN=genet.example.com" \
  -addext "subjectAltName=DNS:genet.example.com"

# 2. 创建 TLS Secret
kubectl create secret tls genet-tls \
  --cert=genet.crt \
  --key=genet.key \
  -n genet-system

# 3. 部署 Genet
helm upgrade --install genet ./helm/genet \
  --set ingress.tls.enabled=true \
  --set ingress.tls.secretName=genet-tls

# 4. 配置 K8s API Server 信任该证书
# --oidc-ca-file=/path/to/genet.crt
```

### 2. 配置 Kubernetes API Server

需要在 kube-apiserver 中添加 OIDC 参数。根据你的 K8s 部署方式选择相应的配置方法。

#### kubeadm 部署

编辑 `/etc/kubernetes/manifests/kube-apiserver.yaml`：

```yaml
spec:
  containers:
  - command:
    - kube-apiserver
    # ... 其他参数 ...
    - --oidc-issuer-url=https://genet.example.com
    - --oidc-client-id=kubernetes
    - --oidc-username-claim=preferred_username
    - --oidc-username-prefix=-
    - --oidc-groups-claim=groups
    - --oidc-groups-prefix=-
    # 如果 Genet 使用自签名证书，需要添加 CA
    # - --oidc-ca-file=/etc/kubernetes/pki/genet-ca.crt
```

保存后 kubelet 会自动重启 kube-apiserver。

#### RKE / RKE2

编辑 `cluster.yml`：

```yaml
services:
  kube-api:
    extra_args:
      oidc-issuer-url: "https://genet.example.com"
      oidc-client-id: "kubernetes"
      oidc-username-claim: "preferred_username"
      oidc-username-prefix: "-"
      oidc-groups-claim: "groups"
      oidc-groups-prefix: "-"
```

然后运行 `rke up` 更新集群。

#### EKS

EKS 原生不支持自定义 OIDC Provider，需要使用 AWS IAM OIDC Provider 或 [kube-oidc-proxy](https://github.com/jetstack/kube-oidc-proxy)。

#### GKE

GKE 支持配置 OIDC：

```bash
gcloud container clusters update CLUSTER_NAME \
  --identity-provider="https://genet.example.com" \
  --identity-provider-client-id="kubernetes"
```

#### AKS

AKS 可以通过 Azure AD 集成或配置 OIDC：

```bash
az aks update -n CLUSTER_NAME -g RESOURCE_GROUP \
  --enable-oidc-issuer
```

### 3. 验证 OIDC 配置

检查 Genet OIDC 端点是否正常：

```bash
# Discovery 端点
curl https://genet.example.com/.well-known/openid-configuration

# JWKS 端点
curl https://genet.example.com/oidc/jwks
```

### 4. 用户使用流程

#### 安装 kubelogin

```bash
# macOS
brew install int128/kubelogin/kubelogin

# Linux
curl -LO https://github.com/int128/kubelogin/releases/latest/download/kubelogin_linux_amd64.zip
unzip kubelogin_linux_amd64.zip
sudo mv kubelogin /usr/local/bin/kubectl-oidc_login

# Windows
choco install kubelogin
```

#### 获取 Kubeconfig

1. 登录 Genet Web UI
2. 点击 "获取 Kubeconfig" 按钮
3. 下载 kubeconfig 文件
4. 保存到 `~/.kube/config` 或设置 `KUBECONFIG` 环境变量

#### 使用 kubectl

```bash
# 首次使用会打开浏览器进行 OIDC 登录
kubectl get pods

# 登录成功后，Token 会被缓存
# 后续命令无需重复登录（直到 Token 过期）
```

## 用户权限模型

当用户首次通过 OIDC 登录时，Genet 会自动创建：

1. **Namespace**: `user-{username}`
2. **Role**: `user-{username}-role`
3. **RoleBinding**: `user-{username}-binding`

用户在自己的 Namespace 内拥有以下权限：

| 资源 | 权限 |
|------|------|
| pods, pods/exec, pods/log, pods/attach, pods/portforward | 完全控制 |
| persistentvolumeclaims | 完全控制 |
| services | 完全控制 |
| configmaps, secrets | 只读 |
| events | 只读 |

## 故障排查

### Token 验证失败

1. 检查 kube-apiserver 日志：
   ```bash
   kubectl logs -n kube-system kube-apiserver-<node>
   ```

2. 验证 JWKS 端点可访问：
   ```bash
   curl -v https://genet.example.com/oidc/jwks
   ```

3. 检查时间同步（JWT 验证对时间敏感）

### 用户无权限

1. 检查 RBAC 是否创建成功：
   ```bash
   kubectl get role,rolebinding -n user-{username}
   ```

2. 检查用户名是否匹配：
   ```bash
   kubectl auth can-i get pods -n user-{username} --as {username}
   ```

### kubelogin 登录失败

1. 检查 kubelogin 版本：
   ```bash
   kubectl oidc-login --version
   ```

2. 清除缓存的 Token：
   ```bash
   rm -rf ~/.kube/cache/oidc-login
   ```

3. 使用 debug 模式：
   ```bash
   kubectl oidc-login get-token \
     --oidc-issuer-url=https://genet.example.com \
     --oidc-client-id=kubernetes \
     -v5
   ```

## 安全建议

1. **HTTPS**: Genet 必须通过 HTTPS 提供服务
2. **RSA 密钥**: 生产环境应配置固定的 RSA 密钥，并妥善保管
3. **Client Secret**: 使用强密码，并通过 Kubernetes Secret 管理
4. **Token 有效期**: 默认 1 小时，可根据安全需求调整
5. **审计日志**: 启用 K8s 审计日志记录用户操作
