package models

import (
	"os"

	"sigs.k8s.io/yaml"
)

// Config 系统配置
type Config struct {
	PodLimitPerUser int             `yaml:"podLimitPerUser" json:"podLimitPerUser"`
	GpuLimitPerUser int             `yaml:"gpuLimitPerUser" json:"gpuLimitPerUser"`
	GPU             GPUConfig       `yaml:"gpu" json:"gpu"`
	UI              UIConfig        `yaml:"ui" json:"ui"`
	Lifecycle       LifecycleConfig `yaml:"lifecycle" json:"lifecycle"`
	Storage         StorageConfig   `yaml:"storage" json:"storage"`
	OAuth           OAuthConfig     `yaml:"oauth" json:"oauth"`
	Proxy           ProxyConfig     `yaml:"proxy" json:"proxy"`
	Registry        RegistryConfig  `yaml:"registry" json:"registry"`
	Images          ImagesConfig    `yaml:"images" json:"images"`
}

// ImagesConfig 系统依赖镜像配置
type ImagesConfig struct {
	Nerdctl string `yaml:"nerdctl" json:"nerdctl"` // nerdctl 镜像，用于 commit 操作
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
	Enabled      bool     `yaml:"enabled" json:"enabled"`
	ProviderURL  string   `yaml:"providerURL" json:"providerURL"`     // OIDC Issuer URL
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
	StorageClass string        `yaml:"storageClass" json:"storageClass"`
	Size         string        `yaml:"size" json:"size"`
	ExtraVolumes []ExtraVolume `yaml:"extraVolumes" json:"extraVolumes"` // 额外的通用存储
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
			DefaultTTLHours: 4,
			MinTTLHours:     1,
			MaxTTLHours:     24,
			EnableJupyter:   false,
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
		OAuth: OAuthConfig{
			Enabled:      false,
			ProviderURL:  "",
			ClientID:     "",
			ClientSecret: "",
			RedirectURL:  "http://localhost:8080/api/auth/callback",
			FrontendURL:  "http://localhost:3000",
			Scopes:       []string{"openid", "profile", "email"},
			JWTSecret:    "genet-jwt-secret-change-in-production",
			CookieDomain: "",
			CookieSecure: false,
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
	}
}

