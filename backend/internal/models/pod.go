package models

import "time"

// PodRequest 创建 Pod 请求
type PodRequest struct {
	Image    string `json:"image" binding:"required"`
	GPUType  string `json:"gpuType"` // GPU 数量为 0 时可选
	GPUCount int    `json:"gpuCount" binding:"min=0,max=8"` // 支持 0 GPU，不使用 required（0 会被认为是空值）
	TTLHours int    `json:"ttlHours" binding:"required,min=1,max=24"`
}

// PodResponse Pod 响应
type PodResponse struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Status      string         `json:"status"`
	Phase       string         `json:"phase"`
	Image       string         `json:"image"`
	GPUType     string         `json:"gpuType"`
	GPUCount    int            `json:"gpuCount"`
	CreatedAt   time.Time      `json:"createdAt"`
	ExpiresAt   time.Time      `json:"expiresAt"`
	NodeIP      string         `json:"nodeIP"`
	Connections ConnectionInfo `json:"connections"`
}

// ConnectionInfo 连接信息
type ConnectionInfo struct {
	SSH  SSHConnection  `json:"ssh"`
	Apps AppConnections `json:"apps"`
}

// SSHConnection SSH 连接信息
type SSHConnection struct {
	Host     string `json:"host"`
	Port     int32  `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// AppConnections 应用连接信息
type AppConnections struct {
	SSHCommand  string `json:"sshCommand"`
	VSCodeURI   string `json:"vscodeURI"`
	XshellURI   string `json:"xshellURI"`
	TerminalURI string `json:"terminalURI"`
}

// PodListResponse Pod 列表响应
type PodListResponse struct {
	Pods  []PodResponse `json:"pods"`
	Quota QuotaInfo     `json:"quota"`
}

// QuotaInfo 配额信息
type QuotaInfo struct {
	PodUsed   int `json:"podUsed"`
	PodLimit  int `json:"podLimit"`
	GpuUsed   int `json:"gpuUsed"`
	GpuLimit  int `json:"gpuLimit"`
}

// ExtendPodRequest 延长 Pod TTL 请求
type ExtendPodRequest struct {
	Hours int `json:"hours" binding:"required,min=1,max=24"`
}

// ConfigResponse 配置响应
type ConfigResponse struct {
	PodLimitPerUser int           `json:"podLimitPerUser"`
	GpuLimitPerUser int           `json:"gpuLimitPerUser"`
	GPUTypes        []GPUType     `json:"gpuTypes"`      // 注意：JSON 字段名用小写
	PresetImages    []PresetImage `json:"presetImages"`
	UI              UIConfig      `json:"ui"`
}

