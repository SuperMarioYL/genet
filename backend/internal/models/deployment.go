package models

import "time"

// DeploymentRequest 创建 Deployment 请求
type DeploymentRequest struct {
	Image      string      `json:"image" binding:"required"`
	GPUType    string      `json:"gpuType"`
	GPUCount   int         `json:"gpuCount" binding:"min=0,max=8"`
	CPU        string      `json:"cpu"`
	Memory     string      `json:"memory"`
	ShmSize    string      `json:"shmSize,omitempty"`
	NodeName   string      `json:"nodeName,omitempty"`
	Name       string      `json:"name,omitempty"`
	Replicas   int         `json:"replicas" binding:"required,min=1,max=8"`
	UserMounts []UserMount `json:"userMounts,omitempty"`
}

// DeploymentResponse Deployment 响应
type DeploymentResponse struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	Namespace         string        `json:"namespace"`
	Managed           bool          `json:"managed"`
	Status            string        `json:"status"`
	Image             string        `json:"image"`
	GPUType           string        `json:"gpuType"`
	GPUCount          int           `json:"gpuCount"`
	CPU               string        `json:"cpu"`
	Memory            string        `json:"memory"`
	Replicas          int32         `json:"replicas"`
	ReadyReplicas     int32         `json:"readyReplicas"`
	CreatedAt         time.Time     `json:"createdAt"`
	Pods              []PodResponse `json:"pods"`
	Suspended         bool          `json:"suspended"`
	SuspendedImage    string        `json:"suspendedImage,omitempty"`
	SuspendedReplicas int32         `json:"suspendedReplicas,omitempty"`
	SuspendedAt       *time.Time    `json:"suspendedAt,omitempty"`
}

// DeploymentListResponse Deployment 列表响应
type DeploymentListResponse struct {
	Items []DeploymentResponse `json:"items"`
}

// OpenAPIDeploymentCreateRequest OpenAPI Deployment 创建请求
type OpenAPIDeploymentCreateRequest struct {
	Image      string      `json:"image" binding:"required"`
	GPUType    string      `json:"gpuType,omitempty"`
	GPUCount   int         `json:"gpuCount" binding:"min=0,max=8"`
	CPU        string      `json:"cpu,omitempty"`
	Memory     string      `json:"memory,omitempty"`
	ShmSize    string      `json:"shmSize,omitempty"`
	NodeName   string      `json:"nodeName,omitempty"`
	Name       string      `json:"name,omitempty"`
	Replicas   int         `json:"replicas" binding:"required,min=1,max=8"`
	UserMounts []UserMount `json:"userMounts,omitempty"`
}
