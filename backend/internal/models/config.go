package models

import (
	"os"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// Config 系统配置
type Config struct {
	PodLimitPerUser int              `yaml:"podLimitPerUser" json:"podLimitPerUser"`
	GpuLimitPerUser int              `yaml:"gpuLimitPerUser" json:"gpuLimitPerUser"`
	GPU             GPUConfig        `yaml:"gpu" json:"gpu"`
	UI              UIConfig         `yaml:"ui" json:"ui"`
	Lifecycle       LifecycleConfig  `yaml:"lifecycle" json:"lifecycle"`
	Storage         StorageConfig    `yaml:"storage" json:"storage"`
	Pod             PodConfig        `yaml:"pod" json:"pod"`
	OAuth           OAuthConfig      `yaml:"oauth" json:"oauth"`
	Proxy           ProxyConfig      `yaml:"proxy" json:"proxy"`
	Registry        RegistryConfig   `yaml:"registry" json:"registry"`
	Images          ImagesConfig     `yaml:"images" json:"images"`
	Kubernetes      KubernetesConfig `yaml:"kubernetes" json:"kubernetes"`
}

// ImagesConfig 系统依赖镜像配置
type ImagesConfig struct {
	Nerdctl string `yaml:"nerdctl" json:"nerdctl"` // nerdctl 镜像，用于 commit 操作
}

// KubernetesConfig Kubernetes 客户端配置
type KubernetesConfig struct {
	DisableProxy bool `yaml:"disableProxy" json:"disableProxy"` // 禁用 HTTP/HTTPS 代理（解决 Windows 代理冲突）
	Timeout      int  `yaml:"timeout" json:"timeout"`           // API 请求超时时间（秒），默认 30
}

// RegistryConfig 镜像仓库配置
type RegistryConfig struct {
	URL      string `yaml:"url" json:"url"`           // 镜像仓库地址
	Username string `yaml:"username" json:"username"` // 仓库用户名
	Password string `yaml:"password" json:"password"` // 仓库密码
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	HTTPProxy  string   `yaml:"httpProxy" json:"httpProxy"`   // HTTP 代理地址
	HTTPSProxy string   `yaml:"httpsProxy" json:"httpsProxy"` // HTTPS 代理地址
	NoProxy    string   `yaml:"noProxy" json:"noProxy"`       // 不使用代理的地址列表
}

// OAuthConfig OAuth 认证配置
type OAuthConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Mode    string `yaml:"mode" json:"mode"` // "oidc" 或 "oauth"，默认 "oidc"

	// OIDC 模式：只需配置 ProviderURL，自动发现其他端点
	ProviderURL string `yaml:"providerURL" json:"providerURL"` // OIDC Issuer URL

	// OAuth 模式：手动配置端点
	AuthorizationEndpoint string `yaml:"authorizationEndpoint" json:"authorizationEndpoint"` // OAuth 授权端点
	TokenEndpoint         string `yaml:"tokenEndpoint" json:"tokenEndpoint"`                 // OAuth Token 端点
	UserinfoEndpoint      string `yaml:"userinfoEndpoint" json:"userinfoEndpoint"`           // 用户信息端点（可选）

	// 用户信息获取方式
	UserinfoSource     string `yaml:"userinfoSource" json:"userinfoSource"`         // "endpoint", "token", "both"
	UserinfoMethod     string `yaml:"userinfoMethod" json:"userinfoMethod"`         // "get" 或 "post"，默认 "get"
	TokenUsernameClaim string `yaml:"tokenUsernameClaim" json:"tokenUsernameClaim"` // Token 中用户名字段，默认 "preferred_username"
	TokenEmailClaim    string `yaml:"tokenEmailClaim" json:"tokenEmailClaim"`       // Token 中邮箱字段，默认 "email"

	// 公共配置
	ClientID     string   `yaml:"clientID" json:"clientID"`           // OAuth Client ID
	ClientSecret string   `yaml:"clientSecret" json:"clientSecret"`   // OAuth Client Secret
	RedirectURL  string   `yaml:"redirectURL" json:"redirectURL"`     // Callback URL
	FrontendURL  string   `yaml:"frontendURL" json:"frontendURL"`     // 登录成功后重定向的前端 URL
	Scopes       []string `yaml:"scopes" json:"scopes"`               // OAuth Scopes
	JWTSecret    string   `yaml:"jwtSecret" json:"jwtSecret"`         // JWT 签名密钥
	CookieDomain string   `yaml:"cookieDomain" json:"cookieDomain"`   // Cookie Domain
	CookieSecure bool     `yaml:"cookieSecure" json:"cookieSecure"`   // Cookie Secure 标志
}

// StorageConfig 存储配置
type StorageConfig struct {
	StorageClass string `yaml:"storageClass" json:"storageClass"` // 用户 workspace PVC 的 StorageClass
	Size         string `yaml:"size" json:"size"`                 // 用户 workspace PVC 的大小
	// 注意：ExtraVolumes 已废弃，请使用 Pod.ExtraVolumes 和 Pod.ExtraVolumeMounts（K8s 原生格式）
	ExtraVolumes []ExtraVolume `yaml:"extraVolumes,omitempty" json:"extraVolumes,omitempty"` // 废弃：请使用 pod.extraVolumes
}

// ExtraVolume 额外存储配置
type ExtraVolume struct {
	Name      string `yaml:"name" json:"name"`           // 卷名称
	MountPath string `yaml:"mountPath" json:"mountPath"` // 挂载路径
	ReadOnly  bool   `yaml:"readOnly" json:"readOnly"`   // 是否只读
	// 以下三种类型只能选一种
	PVC      string `yaml:"pvc" json:"pvc"`           // PVC 名称
	HostPath string `yaml:"hostPath" json:"hostPath"` // 主机路径
	NFS      *NFS   `yaml:"nfs" json:"nfs"`           // NFS 配置
}

// NFS NFS 存储配置
type NFS struct {
	Server string `yaml:"server" json:"server"` // NFS 服务器地址
	Path   string `yaml:"path" json:"path"`     // NFS 路径
}

// PodConfig Pod 配置（使用 K8s 原生格式）
type PodConfig struct {
	// Resources 资源配置（K8s 原生格式）
	Resources *corev1.ResourceRequirements `yaml:"resources,omitempty" json:"resources,omitempty"`

	// SecurityContext 安全上下文（K8s 原生格式）
	SecurityContext *corev1.SecurityContext `yaml:"securityContext,omitempty" json:"securityContext,omitempty"`

	// NodeSelector 节点选择器（K8s 原生格式）
	NodeSelector map[string]string `yaml:"nodeSelector,omitempty" json:"nodeSelector,omitempty"`

	// Affinity 亲和性调度（K8s 原生格式）
	Affinity *corev1.Affinity `yaml:"affinity,omitempty" json:"affinity,omitempty"`

	// HostNetwork 使用主机网络
	HostNetwork bool `yaml:"hostNetwork" json:"hostNetwork"`

	// DNSPolicy DNS 策略（K8s 原生格式）
	// 可选值: ClusterFirst, ClusterFirstWithHostNet, Default, None
	// 当 hostNetwork=true 时，推荐使用 ClusterFirstWithHostNet
	DNSPolicy corev1.DNSPolicy `yaml:"dnsPolicy,omitempty" json:"dnsPolicy,omitempty"`

	// DNSConfig 自定义 DNS 配置（K8s 原生格式）
	// 当 DNSPolicy 为 None 时必须配置
	DNSConfig *corev1.PodDNSConfig `yaml:"dnsConfig,omitempty" json:"dnsConfig,omitempty"`

	// ExtraVolumes 额外的 Volume 配置（K8s 原生格式）
	// 注意：用户的 workspace PVC 会自动添加，这里配置额外的存储卷
	ExtraVolumes []corev1.Volume `yaml:"extraVolumes,omitempty" json:"extraVolumes,omitempty"`

	// ExtraVolumeMounts 额外的 VolumeMount 配置（K8s 原生格式）
	// 需要与 ExtraVolumes 配合使用
	ExtraVolumeMounts []corev1.VolumeMount `yaml:"extraVolumeMounts,omitempty" json:"extraVolumeMounts,omitempty"`

	// StartupScript 容器启动脚本模板
	// 可用变量: {{.SSHPort}}, {{.Password}}, {{.ProxyScript}}
	// 如果为空，使用默认脚本（假设镜像已有 sshd）
	StartupScript string `yaml:"startupScript,omitempty" json:"startupScript,omitempty"`

	// EnableInitContainer 是否启用 InitContainer 预装 SSH 工具
	// 启用后可支持任意基础镜像（无需镜像内置 sshd）
	EnableInitContainer bool `yaml:"enableInitContainer" json:"enableInitContainer"`

	// SSHToolsImage InitContainer 使用的 SSH 工具镜像
	// 包含静态编译的 sshd, sftp-server, ssh-keygen
	SSHToolsImage string `yaml:"sshToolsImage,omitempty" json:"sshToolsImage,omitempty"`
}

// GPUConfig GPU 相关配置
type GPUConfig struct {
	AvailableTypes []GPUType     `yaml:"availableTypes"`
	PresetImages   []PresetImage `yaml:"presetImages"`
}

// GPUType GPU 类型
type GPUType struct {
	Name         string            `yaml:"name" json:"name"`
	ResourceName string            `yaml:"resourceName" json:"resourceName"`
	NodeSelector map[string]string `yaml:"nodeSelector" json:"nodeSelector"`
}

// PresetImage 预设镜像
type PresetImage struct {
	Name        string `yaml:"name" json:"name"`
	Image       string `yaml:"image" json:"image"`
	Description string `yaml:"description" json:"description"`
}

// UIConfig UI 相关配置
type UIConfig struct {
	DefaultTTLHours   int  `yaml:"defaultTTLHours" json:"defaultTTLHours"`
	MinTTLHours       int  `yaml:"minTTLHours" json:"minTTLHours"`
	MaxTTLHours       int  `yaml:"maxTTLHours" json:"maxTTLHours"`
	EnableJupyter     bool `yaml:"enableJupyter" json:"enableJupyter"`
	EnableCustomImage bool `yaml:"enableCustomImage" json:"enableCustomImage"`
}

// LifecycleConfig 生命周期配置
type LifecycleConfig struct {
	AutoDeleteTime string `yaml:"autoDeleteTime"`
	Timezone       string `yaml:"timezone"`
	WarningThresholdHours int `yaml:"warningThresholdHours"`
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		PodLimitPerUser: 5,
		GpuLimitPerUser: 8,
		GPU: GPUConfig{
			AvailableTypes: []GPUType{
				{
					Name:         "NVIDIA A100",
					ResourceName: "nvidia.com/gpu",
					NodeSelector: map[string]string{"gpu-type": "a100"},
				},
			},
			PresetImages: []PresetImage{
				{
					Name:        "CUDA 12.0",
					Image:       "nvidia/cuda:12.0.0-base-ubuntu22.04",
					Description: "NVIDIA CUDA 12.0 基础镜像",
				},
			},
		},
		UI: UIConfig{
			DefaultTTLHours:   4,
			MinTTLHours:       1,
			MaxTTLHours:       24,
			EnableJupyter:     false,
			EnableCustomImage: true,
		},
		Lifecycle: LifecycleConfig{
			AutoDeleteTime:        "23:00",
			Timezone:              "Asia/Shanghai",
			WarningThresholdHours: 1,
		},
		Storage: StorageConfig{
			StorageClass: "hostpath",
			Size:         "50Gi",
			ExtraVolumes: []ExtraVolume{},
		},
		Pod: PodConfig{
			HostNetwork:         true,
			DNSPolicy:           corev1.DNSClusterFirstWithHostNet, // hostNetwork=true 时推荐
			Resources:           nil,                               // 使用 nil 表示使用硬编码的默认值
			EnableInitContainer: false,                             // 默认不启用 InitContainer
			SSHToolsImage:       "",                                // SSH 工具镜像
			StartupScript: `#!/bin/bash
set -e

echo "=== Starting Genet Pod ==="

GENET_DIR=/workspace/.genet

# 创建必要目录
mkdir -p /run/sshd /workspace

# 持久化 VS Code Server 目录（避免每次连接重新下载）
mkdir -p /workspace/.vscode-server
rm -rf /root/.vscode-server 2>/dev/null || true
ln -sf /workspace/.vscode-server /root/.vscode-server
echo "VS Code Server directory linked to /workspace/.vscode-server"

{{.ProxyScript}}

# 设置 root 密码
echo "root:{{.Password}}" | chpasswd

# 检测使用 InitContainer 安装的 sshd 还是系统 sshd
if [ -x "$GENET_DIR/bin/sshd" ]; then
    echo "Using InitContainer SSH tools from $GENET_DIR/bin/"
    SSHD_BIN="$GENET_DIR/bin/sshd"
    SFTP_BIN="$GENET_DIR/bin/sftp-server"
    SSH_KEYGEN="$GENET_DIR/bin/ssh-keygen"
    SSH_CONFIG_DIR="$GENET_DIR/etc"
    SSH_HOST_KEY_DIR="$GENET_DIR/etc/ssh"
else
    echo "Using system SSH tools"
    SSHD_BIN="/usr/sbin/sshd"
    SFTP_BIN="/usr/lib/ssh/sftp-server"
    SSH_KEYGEN="ssh-keygen"
    SSH_CONFIG_DIR="/etc/ssh"
    SSH_HOST_KEY_DIR="/etc/ssh"
    # 生成 SSH host keys（如果不存在）
    $SSH_KEYGEN -A 2>/dev/null || true
fi

# 创建 sshd 配置
mkdir -p "$SSH_CONFIG_DIR"
cat > "$SSH_CONFIG_DIR/sshd_config.genet" << SSHEOF
Port {{.SSHPort}}
PermitRootLogin yes
PasswordAuthentication yes
PubkeyAuthentication yes
ChallengeResponseAuthentication no
UsePAM no
Subsystem sftp $SFTP_BIN
HostKey $SSH_HOST_KEY_DIR/ssh_host_rsa_key
HostKey $SSH_HOST_KEY_DIR/ssh_host_ecdsa_key
HostKey $SSH_HOST_KEY_DIR/ssh_host_ed25519_key
SSHEOF

# 启动 SSH 服务
echo "Starting sshd on port {{.SSHPort}}..."
$SSHD_BIN -f "$SSH_CONFIG_DIR/sshd_config.genet" -D &
SSHD_PID=$!
sleep 2

# 检查 sshd 是否启动成功
if kill -0 $SSHD_PID 2>/dev/null; then
    echo "sshd started successfully (PID: $SSHD_PID)"
else
    echo "WARNING: sshd may have failed to start"
fi

# 显示 GPU 信息（如果有）
if command -v nvidia-smi &> /dev/null; then
    echo "===== GPU Information ====="
    nvidia-smi || true
else
    echo "===== CPU Only Mode ====="
fi

# 显示连接信息
echo ""
echo "============================================"
echo "Pod is ready!"
echo "SSH Port: {{.SSHPort}}"
echo "Connect: ssh root@<node-ip> -p {{.SSHPort}}"
echo "Password: {{.Password}}"
echo "============================================"

# 保持容器运行
tail -f /dev/null
`,
		},
		OAuth: OAuthConfig{
			Enabled:               false,
			Mode:                  "oidc", // 默认 OIDC 模式
			ProviderURL:           "",
			AuthorizationEndpoint: "",
			TokenEndpoint:         "",
			UserinfoEndpoint:      "",
			UserinfoSource:        "endpoint",           // 默认从 endpoint 获取
			UserinfoMethod:        "get",                // 默认 GET 方式
			TokenUsernameClaim:    "preferred_username", // OIDC 标准字段
			TokenEmailClaim:       "email",
			ClientID:              "",
			ClientSecret:          "",
			RedirectURL:           "http://localhost:8080/api/auth/callback",
			FrontendURL:           "http://localhost:3000",
			Scopes:                []string{"openid", "profile", "email"},
			JWTSecret:             "genet-jwt-secret-change-in-production",
			CookieDomain:          "",
			CookieSecure:          false,
		},
		Proxy: ProxyConfig{
			HTTPProxy:  "",
			HTTPSProxy: "",
			NoProxy:    "localhost,127.0.0.1,.cluster.local",
		},
		Registry: RegistryConfig{
			URL:      "",
			Username: "",
			Password: "",
		},
		Images: ImagesConfig{
			Nerdctl: "ghcr.io/containerd/nerdctl:v1.7.0",
		},
		Kubernetes: KubernetesConfig{
			DisableProxy: true, // 默认禁用代理，避免 Windows 代理冲突
			Timeout:      30,   // 默认 30 秒超时
		},
	}
}

