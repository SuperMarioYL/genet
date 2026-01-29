package cleanup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/logger"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodCleaner Pod 清理器
type PodCleaner struct {
	k8sClient *k8s.Client
	config    *models.Config
	log       *zap.Logger
}

// NewPodCleaner 创建 Pod 清理器
func NewPodCleaner(k8sClient *k8s.Client, config *models.Config) *PodCleaner {
	return &PodCleaner{
		k8sClient: k8sClient,
		config:    config,
		log:       logger.Named("cleanup"),
	}
}

// isPodProtected 检查 Pod 是否受保护
// 如果 Pod 有 genet.io/protected-until 注解且时间未过期，返回 true
func (c *PodCleaner) isPodProtected(annotations map[string]string) bool {
	protectedStr, ok := annotations["genet.io/protected-until"]
	if !ok || protectedStr == "" {
		return false
	}

	protectedUntil, err := time.Parse(time.RFC3339, protectedStr)
	if err != nil {
		c.log.Warn("Invalid protected-until annotation",
			zap.String("value", protectedStr),
			zap.Error(err))
		return false
	}

	// 使用 UTC 统一比较，避免时区问题
	now := time.Now().UTC()
	protectedUntilUTC := protectedUntil.UTC()
	return now.Before(protectedUntilUTC) || now.Equal(protectedUntilUTC)
}

// CleanupAllPods 清理所有用户 Pod
// 由 CronJob 在每天 23:00 触发，删除所有未受保护的用户 Pod
func (c *PodCleaner) CleanupAllPods() error {
	c.log.Info("Starting pod cleanup")

	ctx := context.Background()
	clientset := c.k8sClient.GetClientset()

	// 列出所有用户 namespace
	namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "genet.io/managed=true",
	})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}

	totalChecked := 0
	totalDeleted := 0
	totalProtected := 0

	// 遍历每个用户 namespace
	for _, ns := range namespaces.Items {
		if !strings.HasPrefix(ns.Name, "user-") {
			continue
		}

		// 列出该 namespace 下的所有 Pod
		pods, err := c.k8sClient.ListPods(ctx, ns.Name)
		if err != nil {
			c.log.Error("Error listing pods",
				zap.String("namespace", ns.Name),
				zap.Error(err))
			continue
		}

		totalChecked += len(pods)

		// 删除每个未受保护的 Pod
		for _, pod := range pods {
			// 检查是否受保护
			if c.isPodProtected(pod.Annotations) {
				protectedUntil := pod.Annotations["genet.io/protected-until"]
				c.log.Info("Skipping protected pod",
					zap.String("pod", pod.Name),
					zap.String("namespace", ns.Name),
					zap.String("protectedUntil", protectedUntil))
				totalProtected++
				continue
			}

			c.log.Info("Deleting pod",
				zap.String("pod", pod.Name),
				zap.String("namespace", ns.Name),
				zap.String("reason", "scheduled cleanup"))

			err := c.k8sClient.DeletePod(ctx, ns.Name, pod.Name)
			if err != nil {
				c.log.Error("Error deleting pod",
					zap.String("pod", pod.Name),
					zap.Error(err))
			} else {
				totalDeleted++
				c.log.Info("Successfully deleted pod",
					zap.String("pod", pod.Name))
			}
		}
	}

	c.log.Info("Cleanup complete",
		zap.Int("checked", totalChecked),
		zap.Int("deleted", totalDeleted),
		zap.Int("protected", totalProtected))
	return nil
}
