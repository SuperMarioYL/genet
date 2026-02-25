package k8s

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsureNamespace 确保命名空间存在，不存在则创建
func (c *Client) EnsureNamespace(ctx context.Context, namespace string) error {
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		// 命名空间已存在时，也要确保配额与当前配置一致
		if c.shouldManageNamespaceQuota(namespace) {
			if err := c.ensureNamespaceResourceQuota(ctx, namespace); err != nil {
				return fmt.Errorf("更新命名空间配额失败: %w", err)
			}
		}
		return nil
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("获取命名空间失败: %w", err)
	}

	// 创建命名空间
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"genet.io/managed": "true",
			},
		},
	}

	_, err = c.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("创建命名空间失败: %w", err)
	}

	// 新建命名空间后创建默认配额
	if c.shouldManageNamespaceQuota(namespace) {
		if err := c.ensureNamespaceResourceQuota(ctx, namespace); err != nil {
			return fmt.Errorf("创建命名空间配额失败: %w", err)
		}
	}

	return nil
}

func (c *Client) shouldManageNamespaceQuota(namespace string) bool {
	// 仅对用户命名空间自动管理配额，避免影响系统/开放 API 命名空间
	return strings.HasPrefix(namespace, "user-")
}

func (c *Client) ensureNamespaceResourceQuota(ctx context.Context, namespace string) error {
	const quotaName = "genet-user-quota"

	hard := corev1.ResourceList{
		corev1.ResourcePods:                            resource.MustParse(strconv.Itoa(sanitizeQuotaLimit(c.config.PodLimitPerUser))),
		corev1.ResourceName("requests.nvidia.com/gpu"): resource.MustParse(strconv.Itoa(sanitizeQuotaLimit(c.config.GpuLimitPerUser))),
	}

	for _, resName := range c.getAscendResourceNames() {
		quotaKey := corev1.ResourceName(fmt.Sprintf("requests.%s", resName))
		hard[quotaKey] = resource.MustParse(strconv.Itoa(sanitizeQuotaLimit(c.config.GpuLimitPerUser)))
	}

	existing, err := c.clientset.CoreV1().ResourceQuotas(namespace).Get(ctx, quotaName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("获取 ResourceQuota 失败: %w", err)
		}

		rq := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      quotaName,
				Namespace: namespace,
				Labels: map[string]string{
					"genet.io/managed": "true",
				},
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: hard,
			},
		}

		if _, err := c.clientset.CoreV1().ResourceQuotas(namespace).Create(ctx, rq, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("创建 ResourceQuota 失败: %w", err)
		}
		return nil
	}

	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels["genet.io/managed"] = "true"
	existing.Spec.Hard = hard

	if _, err := c.clientset.CoreV1().ResourceQuotas(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("更新 ResourceQuota 失败: %w", err)
	}

	return nil
}

func (c *Client) getAscendResourceNames() []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)

	for _, gpuType := range c.config.GPU.AvailableTypes {
		resourceName := strings.TrimSpace(gpuType.ResourceName)
		if resourceName == "" || resourceName == "nvidia.com/gpu" {
			continue
		}

		typeLower := strings.ToLower(strings.TrimSpace(gpuType.Type))
		resourceLower := strings.ToLower(resourceName)
		if typeLower != "ascend" &&
			!strings.Contains(resourceLower, "ascend") &&
			!strings.Contains(resourceLower, "huawei") &&
			!strings.Contains(resourceLower, "npu") {
			continue
		}

		if _, ok := seen[resourceName]; ok {
			continue
		}
		seen[resourceName] = struct{}{}
		result = append(result, resourceName)
	}

	sort.Strings(result)
	return result
}

func sanitizeQuotaLimit(limit int) int {
	if limit < 0 {
		return 0
	}
	return limit
}

// SyncUserNamespaceQuotas 全量同步所有用户命名空间的 ResourceQuota
// 用于配置变更后批量刷新，确保已有命名空间的配额跟随最新 values。
func (c *Client) SyncUserNamespaceQuotas(ctx context.Context) error {
	namespaces, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "genet.io/managed=true",
	})
	if err != nil {
		return fmt.Errorf("列出命名空间失败: %w", err)
	}

	var failed []string
	for _, ns := range namespaces.Items {
		namespace := ns.Name
		if !c.shouldManageNamespaceQuota(namespace) {
			continue
		}
		if err := c.ensureNamespaceResourceQuota(ctx, namespace); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", namespace, err))
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("同步部分命名空间配额失败: %s", strings.Join(failed, "; "))
	}

	return nil
}
