package k8s

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BuildConfigMapFromOpenAPIRequest(namespace, ownerUser string, req *models.OpenAPIConfigMapRequest) (*corev1.ConfigMap, error) {
	if req == nil {
		return nil, fmt.Errorf("configmap 请求不能为空")
	}

	binaryData := make(map[string][]byte, len(req.BinaryData))
	for key, value := range req.BinaryData {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, fmt.Errorf("binaryData %s 不是合法 base64: %w", key, err)
		}
		binaryData[key] = decoded
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: namespace,
			Labels: map[string]string{
				"genet.io/open-api":      "true",
				"genet.io/managed":       "true",
				"genet.io/openapi-owner": ownerUser,
			},
			Annotations: req.Annotations,
		},
		Data:       req.Data,
		BinaryData: binaryData,
	}
	if req.Immutable != nil {
		cm.Immutable = req.Immutable
	}

	return cm, nil
}

func (c *Client) CreateConfigMap(ctx context.Context, configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return c.clientset.CoreV1().ConfigMaps(configMap.Namespace).Create(ctx, configMap, metav1.CreateOptions{})
}

func (c *Client) ListConfigMaps(ctx context.Context, namespace, labelSelector string) (*corev1.ConfigMapList, error) {
	return c.clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

func (c *Client) GetConfigMap(ctx context.Context, namespace, name string) (*corev1.ConfigMap, error) {
	return c.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) UpdateConfigMap(ctx context.Context, configMap *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return c.clientset.CoreV1().ConfigMaps(configMap.Namespace).Update(ctx, configMap, metav1.UpdateOptions{})
}

func (c *Client) DeleteConfigMap(ctx context.Context, namespace, name string) error {
	return c.clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}
