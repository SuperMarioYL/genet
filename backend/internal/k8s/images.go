package k8s

import (
	"context"
	"encoding/json"
	"time"

	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// UserImagesConfigMapName ConfigMap 名称
	UserImagesConfigMapName = "genet-user-images"
	// UserImagesDataKey ConfigMap 中存储镜像列表的 key
	UserImagesDataKey = "images.json"
)

// GetUserImages 获取用户保存的镜像列表
func (c *Client) GetUserImages(ctx context.Context, namespace string) (*models.UserImageList, error) {
	cm, err := c.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, UserImagesConfigMapName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return &models.UserImageList{Images: []models.UserSavedImage{}}, nil
		}
		return nil, err
	}

	data, ok := cm.Data[UserImagesDataKey]
	if !ok || data == "" {
		return &models.UserImageList{Images: []models.UserSavedImage{}}, nil
	}

	var imageList models.UserImageList
	if err := json.Unmarshal([]byte(data), &imageList); err != nil {
		c.log.Warn("Failed to unmarshal user images", zap.String("namespace", namespace), zap.Error(err))
		return &models.UserImageList{Images: []models.UserSavedImage{}}, nil
	}

	return &imageList, nil
}

// SaveUserImage 保存用户镜像记录
func (c *Client) SaveUserImage(ctx context.Context, namespace string, image *models.UserSavedImage) error {
	// 设置保存时间
	if image.SavedAt.IsZero() {
		image.SavedAt = time.Now()
	}

	// 获取现有列表
	imageList, err := c.GetUserImages(ctx, namespace)
	if err != nil {
		return err
	}

	// 检查是否已存在相同镜像名（去重）
	for i, existing := range imageList.Images {
		if existing.Image == image.Image {
			// 更新已有记录
			imageList.Images[i] = *image
			return c.saveUserImageList(ctx, namespace, imageList)
		}
	}

	// 添加到列表头部（最新在前）
	imageList.Images = append([]models.UserSavedImage{*image}, imageList.Images...)

	return c.saveUserImageList(ctx, namespace, imageList)
}

// DeleteUserImage 删除用户镜像记录
func (c *Client) DeleteUserImage(ctx context.Context, namespace, imageName string) error {
	imageList, err := c.GetUserImages(ctx, namespace)
	if err != nil {
		return err
	}

	// 过滤掉要删除的镜像
	filtered := make([]models.UserSavedImage, 0, len(imageList.Images))
	for _, img := range imageList.Images {
		if img.Image != imageName {
			filtered = append(filtered, img)
		}
	}
	imageList.Images = filtered

	return c.saveUserImageList(ctx, namespace, imageList)
}

// saveUserImageList 保存镜像列表到 ConfigMap
func (c *Client) saveUserImageList(ctx context.Context, namespace string, imageList *models.UserImageList) error {
	data, err := json.Marshal(imageList)
	if err != nil {
		return err
	}

	cm, err := c.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, UserImagesConfigMapName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// 创建新的 ConfigMap
			newCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      UserImagesConfigMapName,
					Namespace: namespace,
					Labels: map[string]string{
						"genet.io/type":    "user-images",
						"genet.io/managed": "true",
					},
				},
				Data: map[string]string{
					UserImagesDataKey: string(data),
				},
			}
			_, err = c.clientset.CoreV1().ConfigMaps(namespace).Create(ctx, newCM, metav1.CreateOptions{})
			return err
		}
		return err
	}

	// 更新现有 ConfigMap
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[UserImagesDataKey] = string(data)

	_, err = c.clientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
	return err
}
