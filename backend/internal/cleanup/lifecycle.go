package cleanup

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodCleaner Pod 清理器
type PodCleaner struct {
	k8sClient *k8s.Client
	config    *models.Config
}

// NewPodCleaner 创建 Pod 清理器
func NewPodCleaner(k8sClient *k8s.Client, config *models.Config) *PodCleaner {
	return &PodCleaner{
		k8sClient: k8sClient,
		config:    config,
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
		log.Printf("Invalid protected-until annotation: %s", protectedStr)
		return false
	}

	// 如果保护截止时间 >= 当前时间，则受保护
	return time.Now().Before(protectedUntil) || time.Now().Equal(protectedUntil)
}

// CleanupAllPods 清理所有用户 Pod
// 由 CronJob 在每天 23:00 触发，删除所有未受保护的用户 Pod
func (c *PodCleaner) CleanupAllPods() error {
	log.Println("Starting pod cleanup...")

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
			log.Printf("Error listing pods in namespace %s: %v", ns.Name, err)
			continue
		}

		totalChecked += len(pods)

		// 删除每个未受保护的 Pod
		for _, pod := range pods {
			// 检查是否受保护
			if c.isPodProtected(pod.Annotations) {
				protectedUntil := pod.Annotations["genet.io/protected-until"]
				log.Printf("Skipping protected pod %s in namespace %s (protected until %s)",
					pod.Name, ns.Name, protectedUntil)
				totalProtected++
				continue
			}

			log.Printf("Deleting pod %s in namespace %s (scheduled cleanup)", pod.Name, ns.Name)

			err := c.k8sClient.DeletePod(ctx, ns.Name, pod.Name)
			if err != nil {
				log.Printf("Error deleting pod %s: %v", pod.Name, err)
			} else {
				totalDeleted++
				log.Printf("Successfully deleted pod %s", pod.Name)
			}
		}
	}

	log.Printf("Cleanup complete: checked %d pods, deleted %d, protected %d",
		totalChecked, totalDeleted, totalProtected)
	return nil
}
