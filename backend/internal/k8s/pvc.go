package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsurePVC 确保 PVC 存在，不存在则创建
func (c *Client) EnsurePVC(ctx context.Context, namespace, username, storageClass, size string) error {
	pvcName := fmt.Sprintf("%s-workspace", username)

	// 检查是否已存在
	_, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err == nil {
		return nil // 已存在，直接返回
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("获取 PVC 失败: %w", err)
	}

	// 创建新 PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"genet.io/user":    username,
				"genet.io/managed": "true",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}

	// 设置 StorageClass（如果指定）
	if storageClass != "" {
		pvc.Spec.StorageClassName = &storageClass
	}

	_, err = c.clientset.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("创建 PVC 失败: %w", err)
	}

	return nil
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

