package models

import "time"

type OpenAPIEnvVar struct {
	Name  string `json:"name" binding:"required"`
	Value string `json:"value,omitempty"`
}

type OpenAPIJobRequest struct {
	Name                    string            `json:"name" binding:"required"`
	Image                   string            `json:"image" binding:"required"`
	Command                 []string          `json:"command,omitempty"`
	Args                    []string          `json:"args,omitempty"`
	Env                     []OpenAPIEnvVar   `json:"env,omitempty"`
	WorkingDir              string            `json:"workingDir,omitempty"`
	GPUType                 string            `json:"gpuType,omitempty"`
	GPUCount                int               `json:"gpuCount,omitempty" binding:"min=0,max=8"`
	CPU                     string            `json:"cpu,omitempty"`
	Memory                  string            `json:"memory,omitempty"`
	ShmSize                 string            `json:"shmSize,omitempty"`
	NodeName                string            `json:"nodeName,omitempty"`
	GPUDevices              []int             `json:"gpuDevices,omitempty"`
	UserMounts              []UserMount       `json:"userMounts,omitempty"`
	Parallelism             *int32            `json:"parallelism,omitempty"`
	Completions             *int32            `json:"completions,omitempty"`
	BackoffLimit            *int32            `json:"backoffLimit,omitempty"`
	TTLSecondsAfterFinished *int32            `json:"ttlSecondsAfterFinished,omitempty"`
	RestartPolicy           string            `json:"restartPolicy,omitempty"`
	Annotations             map[string]string `json:"annotations,omitempty"`
}

type OpenAPIJobResponse struct {
	Name           string     `json:"name"`
	Namespace      string     `json:"namespace"`
	Image          string     `json:"image"`
	Parallelism    *int32     `json:"parallelism,omitempty"`
	Completions    *int32     `json:"completions,omitempty"`
	Active         int32      `json:"active"`
	Succeeded      int32      `json:"succeeded"`
	Failed         int32      `json:"failed"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	CompletionTime *time.Time `json:"completionTime,omitempty"`
}

type OpenAPIJobListResponse struct {
	Jobs []OpenAPIJobResponse `json:"jobs"`
}
