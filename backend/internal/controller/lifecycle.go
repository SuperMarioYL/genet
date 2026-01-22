package controller

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LifecycleController 生命周期控制器
type LifecycleController struct {
	k8sClient *k8s.Client
	config    *models.Config
	location  *time.Location
}

// NewLifecycleController 创建生命周期控制器
func NewLifecycleController(k8sClient *k8s.Client, config *models.Config) *LifecycleController {
	// 加载时区
	location, err := time.LoadLocation(config.Lifecycle.Timezone)
	if err != nil {
		log.Printf("Warning: Failed to load timezone %s, using UTC: %v", config.Lifecycle.Timezone, err)
		location = time.UTC
	}

	return &LifecycleController{
		k8sClient: k8sClient,
		config:    config,
		location:  location,
	}
}

// ReconcileAll 协调所有 Pod
func (c *LifecycleController) ReconcileAll() error {
	log.Println("Starting reconciliation...")

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

		// 检查每个 Pod
		for _, pod := range pods {
			shouldDelete, reason := c.shouldDeletePod(&pod)
			if shouldDelete {
				log.Printf("Deleting pod %s in namespace %s: %s", pod.Name, ns.Name, reason)

				err := c.k8sClient.DeletePod(ctx, ns.Name, pod.Name)
				if err != nil {
					log.Printf("Error deleting pod %s: %v", pod.Name, err)
				} else {
					totalDeleted++
					log.Printf("Successfully deleted pod %s", pod.Name)
				}
			}
		}
	}

	log.Printf("Reconciliation complete: checked %d pods, deleted %d", totalChecked, totalDeleted)
	return nil
}

// shouldDeletePod 判断是否应该删除 Pod
// 只在每日自动删除时间（23:00）删除所有 Pod
func (c *LifecycleController) shouldDeletePod(pod *corev1.Pod) (bool, string) {
	// 检查是否到达自动删除时间（晚上11点）
	if c.isAutoDeleteTime() {
		return true, "reached auto-delete time (23:00)"
	}

	return false, ""
}

// isAutoDeleteTime 判断是否到达自动删除时间
func (c *LifecycleController) isAutoDeleteTime() bool {
	// 获取当前时间（使用配置的时区）
	now := time.Now().In(c.location)

	// 解析自动删除时间
	autoDeleteTime := c.config.Lifecycle.AutoDeleteTime
	parts := strings.Split(autoDeleteTime, ":")
	if len(parts) != 2 {
		log.Printf("Warning: Invalid auto-delete time format: %s", autoDeleteTime)
		return false
	}

	var hour, minute int
	fmt.Sscanf(autoDeleteTime, "%d:%d", &hour, &minute)

	// 检查当前时间是否在自动删除时间窗口内
	deleteTime := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, c.location)
	diff := now.Sub(deleteTime)

	// 如果在删除时间后的 5 分钟内，认为应该删除
	// 扩大窗口从 2 分钟到 5 分钟，以应对 CronJob 可能的延迟
	if diff >= 0 && diff < 5*time.Minute {
		log.Printf("Auto-delete time reached: now=%s, deleteTime=%s, diff=%v",
			now.Format("15:04:05"), deleteTime.Format("15:04:05"), diff)
		return true
	}

	return false
}
