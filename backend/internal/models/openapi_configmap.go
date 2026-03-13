package models

import "time"

type OpenAPIConfigMapRequest struct {
	Name        string            `json:"name" binding:"required"`
	Data        map[string]string `json:"data,omitempty"`
	BinaryData  map[string]string `json:"binaryData,omitempty"`
	Immutable   *bool             `json:"immutable,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type OpenAPIConfigMapSummary struct {
	Name           string    `json:"name"`
	Namespace      string    `json:"namespace"`
	Immutable      bool      `json:"immutable"`
	DataKeys       []string  `json:"dataKeys,omitempty"`
	BinaryDataKeys []string  `json:"binaryDataKeys,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

type OpenAPIConfigMapDetail struct {
	OpenAPIConfigMapSummary
	Data       map[string]string `json:"data,omitempty"`
	BinaryData map[string]string `json:"binaryData,omitempty"`
}

type OpenAPIConfigMapListResponse struct {
	ConfigMaps []OpenAPIConfigMapSummary `json:"configMaps"`
}
