package models

import "time"

const (
	// APIKeyScopeRead 只读权限（GET）
	APIKeyScopeRead = "read"
	// APIKeyScopeWrite 读写权限（GET/POST/PUT/DELETE）
	APIKeyScopeWrite = "write"
)

// APIKeyRecord 管理页维护的 OpenAPI Key 元数据
type APIKeyRecord struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	OwnerUser  string     `json:"ownerUser"`
	Scope      string     `json:"scope"`
	Enabled    bool       `json:"enabled"`
	KeyHash    string     `json:"keyHash"`
	KeyPreview string     `json:"keyPreview,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	CreatedBy  string     `json:"createdBy"`
}
