package k8s

import (
	"context"
	"fmt"
	"strings"

	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"go.uber.org/zap"
)

// EnsureVolumePVCs 根据 storage.volumes 配置，确保所有 PVC 类型的卷对应的 PVC 存在
// 使用 GetPVCName 统一命名逻辑，支持 scope 区分
func (c *Client) EnsureVolumePVCs(ctx context.Context, namespace, userIdentifier, podName string) error {
	storageVolumes := c.config.Storage.GetEffectiveVolumes()

	for _, vol := range storageVolumes {
		// 使用 GetPVCName 统一命名（处理 scope、sanitize 等）
		pvcName := c.GetPVCName(vol, userIdentifier, podName)
		if pvcName == "" {
			continue // HostPath 不需要 PVC
		}

		if err := c.ensureSinglePVC(ctx, namespace, userIdentifier, pvcName, vol); err != nil {
			return fmt.Errorf("确保卷 %s 的 PVC 失败: %w", vol.Name, err)
		}
	}

	return nil
}

// ensureSinglePVC 确保单个 PVC 存在
func (c *Client) ensureSinglePVC(ctx context.Context, namespace, userIdentifier, pvcName string, vol models.StorageVolume) error {
	// 检查是否已存在
	_, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err == nil {
		return nil // 已存在
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("获取 PVC 失败: %w", err)
	}

	// 解析访问模式
	accessMode := c.parseAccessMode(vol.AccessMode)

	// 解析大小
	size := vol.Size
	if size == "" {
		size = "50Gi" // 默认大小
	}

	// 创建 PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"genet.io/user-identifier": userIdentifier,
				"genet.io/volume-name":     vol.Name,
				"genet.io/managed":         "true",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}

	// 设置 StorageClass
	if vol.StorageClass != "" {
		pvc.Spec.StorageClassName = &vol.StorageClass
	}

	c.log.Info("Creating PVC",
		zap.String("name", pvcName),
		zap.String("namespace", namespace),
		zap.String("storageClass", vol.StorageClass),
		zap.String("size", size),
		zap.String("volumeName", vol.Name))

	_, err = c.clientset.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("创建 PVC 失败: %w", err)
	}

	return nil
}

// parseAccessMode 解析 PVC 访问模式字符串
func (c *Client) parseAccessMode(mode string) corev1.PersistentVolumeAccessMode {
	switch strings.ToLower(mode) {
	case "readwriteonce", "rwo":
		return corev1.ReadWriteOnce
	case "readonlymany", "rox":
		return corev1.ReadOnlyMany
	case "readwritemany", "rwx":
		return corev1.ReadWriteMany
	case "readwriteoncepod", "rwop":
		return corev1.ReadWriteOncePod
	default:
		// 默认使用 ReadWriteOnce（最通用，兼容 local-path 等本地存储）
		return corev1.ReadWriteOnce
	}
}

// PVCExists 检查 PVC 是否存在
func (c *Client) PVCExists(ctx context.Context, namespace, name string) bool {
	_, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	return err == nil
}

// DeletePVC 删除 PVC（通常不使用，保留用户数据）
func (c *Client) DeletePVC(ctx context.Context, namespace, name string) error {
	err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("删除 PVC 失败: %w", err)
	}
	return nil
}
