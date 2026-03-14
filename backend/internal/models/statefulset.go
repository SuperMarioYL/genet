package models

import "time"

// StatefulSetRequest 创建 StatefulSet 请求
type StatefulSetRequest struct {
	Image    string `json:"image" binding:"required"`
	GPUType  string `json:"gpuType"`
	GPUCount int    `json:"gpuCount" binding:"min=0,max=8"`
	CPU      string `json:"cpu"`
	Memory   string `json:"memory"`
	ShmSize  string `json:"shmSize,omitempty"`
	NodeName string `json:"nodeName,omitempty"`
	Name     string `json:"name,omitempty"`
	Replicas int    `json:"replicas" binding:"required,min=1,max=8"`

	UserMounts []UserMount `json:"userMounts,omitempty"`
}

// StatefulSetResponse StatefulSet 响应
type StatefulSetResponse struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Namespace        string        `json:"namespace"`
	Status           string        `json:"status"`
	Image            string        `json:"image"`
	GPUType          string        `json:"gpuType"`
	GPUCount         int           `json:"gpuCount"`
	CPU              string        `json:"cpu"`
	Memory           string        `json:"memory"`
	Replicas         int32         `json:"replicas"`
	ReadyReplicas    int32         `json:"readyReplicas"`
	CreatedAt        time.Time     `json:"createdAt"`
	ServiceName      string        `json:"serviceName"`
	Pods             []PodResponse `json:"pods"`
	ProtectedUntil   *time.Time    `json:"protectedUntil,omitempty"`
	ParentConnection string        `json:"parentConnection,omitempty"`
}

// StatefulSetListResponse StatefulSet 列表响应
type StatefulSetListResponse struct {
	Items []StatefulSetResponse `json:"items"`
}

// OpenAPIStatefulSetCreateRequest OpenAPI StatefulSet 创建请求
type OpenAPIStatefulSetCreateRequest struct {
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
