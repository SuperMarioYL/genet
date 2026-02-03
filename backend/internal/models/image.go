package models

import "time"

// UserSavedImage 用户保存的镜像记录
type UserSavedImage struct {
	Image       string    `json:"image"`                 // 完整镜像名
	Description string    `json:"description,omitempty"` // 描述（可选）
	SourcePod   string    `json:"sourcePod,omitempty"`   // 来源 Pod
	SavedAt     time.Time `json:"savedAt"`               // 保存时间
}

// UserImageList 用户镜像列表
type UserImageList struct {
	Images []UserSavedImage `json:"images"`
}

// AddUserImageRequest 添加用户镜像请求
type AddUserImageRequest struct {
	Image       string `json:"image" binding:"required"` // 完整镜像名
	Description string `json:"description,omitempty"`    // 描述（可选）
	SourcePod   string `json:"sourcePod,omitempty"`      // 来源 Pod
}
