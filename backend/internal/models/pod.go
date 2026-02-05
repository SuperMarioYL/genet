package models

import "time"

// PodRequest 创建 Pod 请求
type PodRequest struct {
	Image    string `json:"image" binding:"required"`
	GPUType  string `json:"gpuType"`                        // GPU 数量为 0 时可选
	GPUCount int    `json:"gpuCount" binding:"min=0,max=8"` // 支持 0 GPU，不使用 required（0 会被认为是空值）
	CPU      string `json:"cpu"`                            // CPU 核数，如 "2", "4"
	Memory   string `json:"memory"`                         // 内存大小，如 "4Gi", "8Gi"
	// 高级配置字段
	NodeName   string `json:"nodeName,omitempty"`   // 指定调度节点（可选）
	GPUDevices []int  `json:"gpuDevices,omitempty"` // 指定 GPU 卡编号（可选），如 [0, 2, 5]
	// Pod 名称自定义
	Name string `json:"name,omitempty"` // 自定义 Pod 名称后缀（可选），如 "train", "dev"，为空则使用时间戳
	// 用户自定义挂载（需要管理员开启 storage.allowUserMounts）
	UserMounts []UserMount `json:"userMounts,omitempty"`
}

// UserMount 用户自定义挂载
type UserMount struct {
	HostPath  string `json:"hostPath" binding:"required"`  // 宿主机路径
	MountPath string `json:"mountPath" binding:"required"` // 容器内挂载路径
	ReadOnly  bool   `json:"readOnly,omitempty"`           // 是否只读，默认 false
}

// PodResponse Pod 响应
type PodResponse struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Namespace      string     `json:"namespace"` // Pod 所在的 namespace
	Container      string     `json:"container"` // 主容器名称
	Status         string     `json:"status"`
	Phase          string     `json:"phase"`
	Image          string     `json:"image"`
	GPUType        string     `json:"gpuType"`
	GPUCount       int        `json:"gpuCount"`
	CPU            string     `json:"cpu"`    // CPU 核数
	Memory         string     `json:"memory"` // 内存大小
	CreatedAt      time.Time  `json:"createdAt"`
	NodeIP         string     `json:"nodeIP"`
	ProtectedUntil *time.Time `json:"protectedUntil,omitempty"` // 保护截止时间，nil 表示未保护
}

// PodListResponse Pod 列表响应
type PodListResponse struct {
	Pods  []PodResponse `json:"pods"`
	Quota QuotaInfo     `json:"quota"`
}

// QuotaInfo 配额信息
type QuotaInfo struct {
	PodUsed  int `json:"podUsed"`
	PodLimit int `json:"podLimit"`
	GpuUsed  int `json:"gpuUsed"`
	GpuLimit int `json:"gpuLimit"`
}

// StorageVolumeInfo 存储卷信息（用于前端展示）
type StorageVolumeInfo struct {
	Name        string `json:"name"`                  // 卷名称
	MountPath   string `json:"mountPath"`             // 挂载路径
	Description string `json:"description,omitempty"` // 描述信息
	ReadOnly    bool   `json:"readOnly"`              // 是否只读
	Type        string `json:"type"`                  // 存储类型: pvc | hostpath
	Scope       string `json:"scope,omitempty"`       // 作用域: user | pod
}

// ConfigResponse 配置响应
type ConfigResponse struct {
	PodLimitPerUser int           `json:"podLimitPerUser"`
	GpuLimitPerUser int           `json:"gpuLimitPerUser"`
	GPUTypes        []GPUType     `json:"gpuTypes"` // 注意：JSON 字段名用小写
	PresetImages    []PresetImage `json:"presetImages"`
	UI              UIConfig      `json:"ui"`
	// GPU 调度相关
	GPUSchedulingMode string `json:"gpuSchedulingMode"` // "sharing" | "exclusive"
	MaxPodsPerGPU     int    `json:"maxPodsPerGPU"`     // 每卡最大共享数
	// 存储相关
	AllowUserMounts bool                `json:"allowUserMounts"` // 是否允许用户自定义挂载
	StorageVolumes  []StorageVolumeInfo `json:"storageVolumes"`  // 存储卷信息（用于前端展示）
	// 用户镜像
	UserImages []UserSavedImage `json:"userImages,omitempty"` // 用户保存的镜像列表
	// 镜像仓库
	RegistryURL string `json:"registryUrl,omitempty"` // 镜像仓库地址（用于前端展示）
}
