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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodCleaner Pod 清理器
type PodCleaner struct {
	k8sClient             *k8s.Client
	config                *models.Config
	log                   *zap.Logger
	nowFn                 func() time.Time
	commitWorkloadImageFn func(ctx context.Context, workloadKind, workloadName, namespace, userIdentifier string, pod *corev1.Pod) (string, error)
}

// NewPodCleaner 创建 Pod 清理器
func NewPodCleaner(k8sClient *k8s.Client, config *models.Config) *PodCleaner {
	cleaner := &PodCleaner{
		k8sClient: k8sClient,
		config:    config,
		log:       logger.Named("cleanup"),
		nowFn:     time.Now,
	}
	cleaner.commitWorkloadImageFn = cleaner.commitWorkloadImage
	return cleaner
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
	totalSuspended := 0

	// 遍历每个用户 namespace
	for _, ns := range namespaces.Items {
		if !strings.HasPrefix(ns.Name, "user-") {
			continue
		}

		suspendedCount, err := c.cleanupManagedWorkloads(ctx, ns.Name)
		if err != nil {
			c.log.Warn("Managed workload cleanup completed with errors",
				zap.String("namespace", ns.Name),
				zap.Error(err))
		}
		totalSuspended += suspendedCount

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
				// 删除 scope="pod" 的 PVC（失败仅告警，不影响主流程）
				userIdentifier := pod.Labels["genet.io/user"]
				if userIdentifier == "" && strings.HasPrefix(ns.Name, "user-") {
					userIdentifier = strings.TrimPrefix(ns.Name, "user-")
				}

				if userIdentifier == "" {
					c.log.Warn("Skip deleting scope=pod PVCs: user identifier missing",
						zap.String("pod", pod.Name),
						zap.String("namespace", ns.Name))
				} else if err := c.k8sClient.DeletePodScopedPVCs(ctx, ns.Name, userIdentifier, pod.Name); err != nil {
					c.log.Warn("Failed to delete some scope=pod PVCs",
						zap.String("pod", pod.Name),
						zap.String("namespace", ns.Name),
						zap.String("userIdentifier", userIdentifier),
						zap.Error(err))
				}

				totalDeleted++
				c.log.Info("Successfully deleted pod",
					zap.String("pod", pod.Name))
			}
		}
	}

	c.log.Info("Cleanup complete",
		zap.Int("checked", totalChecked),
		zap.Int("deleted", totalDeleted),
		zap.Int("protected", totalProtected),
		zap.Int("suspended", totalSuspended))
	return nil
}

func (c *PodCleaner) cleanupManagedWorkloads(ctx context.Context, namespace string) (int, error) {
	suspended := 0
	var errs []string

	deployments, err := c.k8sClient.ListDeployments(ctx, namespace)
	if err != nil {
		errs = append(errs, fmt.Sprintf("list deployments: %v", err))
	} else {
		for i := range deployments {
			changed, err := c.suspendDeployment(ctx, namespace, &deployments[i])
			if err != nil {
				errs = append(errs, fmt.Sprintf("suspend deployment %s: %v", deployments[i].Name, err))
				continue
			}
			if changed {
				suspended++
			}
		}
	}

	statefulSets, err := c.k8sClient.ListStatefulSets(ctx, namespace)
	if err != nil {
		errs = append(errs, fmt.Sprintf("list statefulsets: %v", err))
	} else {
		for i := range statefulSets {
			changed, err := c.suspendStatefulSet(ctx, namespace, &statefulSets[i])
			if err != nil {
				errs = append(errs, fmt.Sprintf("suspend statefulset %s: %v", statefulSets[i].Name, err))
				continue
			}
			if changed {
				suspended++
			}
		}
	}

	if len(errs) > 0 {
		return suspended, fmt.Errorf(strings.Join(errs, "; "))
	}
	return suspended, nil
}

func (c *PodCleaner) suspendDeployment(ctx context.Context, namespace string, deploy *appsv1.Deployment) (bool, error) {
	if deploy == nil || deploy.Spec.Replicas == nil || *deploy.Spec.Replicas == 0 {
		return false, nil
	}

	pods, err := c.k8sClient.ListDeploymentPods(ctx, namespace, deploy.Name)
	if err != nil {
		_ = c.recordDeploymentSuspendFailure(ctx, namespace, deploy.Name, err.Error())
		return false, err
	}
	selected, err := k8s.SelectRepresentativePod(pods)
	if err != nil {
		_ = c.recordDeploymentSuspendFailure(ctx, namespace, deploy.Name, err.Error())
		return false, err
	}

	userIdentifier := workloadUserIdentifier(deploy.Labels, namespace)
	image, err := c.commitWorkloadImageFn(ctx, "deployment", deploy.Name, namespace, userIdentifier, selected)
	if err != nil {
		_ = c.recordDeploymentSuspendFailure(ctx, namespace, deploy.Name, err.Error())
		return false, err
	}

	if _, err := c.k8sClient.SuspendDeployment(ctx, namespace, deploy.Name, image, selected.Name, c.nowFn()); err != nil {
		return false, err
	}
	return true, nil
}

func (c *PodCleaner) suspendStatefulSet(ctx context.Context, namespace string, sts *appsv1.StatefulSet) (bool, error) {
	if sts == nil || sts.Spec.Replicas == nil || *sts.Spec.Replicas == 0 {
		return false, nil
	}

	pods, err := c.k8sClient.ListStatefulSetPods(ctx, namespace, sts.Name)
	if err != nil {
		_ = c.recordStatefulSetSuspendFailure(ctx, namespace, sts.Name, err.Error())
		return false, err
	}
	selected, err := k8s.SelectRepresentativePod(pods)
	if err != nil {
		_ = c.recordStatefulSetSuspendFailure(ctx, namespace, sts.Name, err.Error())
		return false, err
	}

	userIdentifier := workloadUserIdentifier(sts.Labels, namespace)
	image, err := c.commitWorkloadImageFn(ctx, "statefulset", sts.Name, namespace, userIdentifier, selected)
	if err != nil {
		_ = c.recordStatefulSetSuspendFailure(ctx, namespace, sts.Name, err.Error())
		return false, err
	}

	if _, err := c.k8sClient.SuspendStatefulSet(ctx, namespace, sts.Name, image, selected.Name, c.nowFn()); err != nil {
		return false, err
	}
	return true, nil
}

func workloadUserIdentifier(labels map[string]string, namespace string) string {
	if labels["genet.io/user"] != "" {
		return labels["genet.io/user"]
	}
	if strings.HasPrefix(namespace, "user-") {
		return strings.TrimPrefix(namespace, "user-")
	}
	return namespace
}

func (c *PodCleaner) buildSuspendImageName(userIdentifier, workloadName string) (string, error) {
	registryURL := strings.TrimSuffix(strings.TrimSpace(c.config.Registry.URL), "/")
	if registryURL == "" {
		return "", fmt.Errorf("registry url not configured")
	}
	tag := c.nowFn().UTC().Format("20060102-150405")
	return fmt.Sprintf("%s/%s/suspend-%s:%s", registryURL, userIdentifier, workloadName, tag), nil
}

func (c *PodCleaner) commitWorkloadImage(ctx context.Context, workloadKind, workloadName, namespace, userIdentifier string, pod *corev1.Pod) (string, error) {
	if pod == nil {
		return "", fmt.Errorf("representative pod missing")
	}

	targetImage, err := c.buildSuspendImageName(userIdentifier, workloadName)
	if err != nil {
		return "", err
	}

	_, err = c.k8sClient.CreateCommitJob(ctx, &k8s.CommitSpec{
		PodName:     pod.Name,
		Namespace:   namespace,
		Username:    userIdentifier,
		TargetImage: targetImage,
		NodeName:    pod.Spec.NodeName,
	})
	if err != nil {
		return "", err
	}

	if err := c.waitForCommitJob(ctx, namespace, pod.Name); err != nil {
		return "", err
	}

	_ = c.k8sClient.SaveUserImage(ctx, namespace, &models.UserSavedImage{
		Image:       targetImage,
		Description: fmt.Sprintf("Scheduled suspend snapshot for %s %s", workloadKind, workloadName),
		SourcePod:   pod.Name,
		SavedAt:     c.nowFn(),
	})

	return targetImage, nil
}

func (c *PodCleaner) waitForCommitJob(ctx context.Context, namespace, podName string) error {
	deadline := c.nowFn().Add(2 * time.Minute)
	for c.nowFn().Before(deadline) {
		status, err := c.k8sClient.GetCommitJobStatus(ctx, namespace, podName)
		if err != nil {
			return err
		}
		if status != nil {
			switch status.Status {
			case "Succeeded":
				return nil
			case "Failed":
				return fmt.Errorf("commit job failed")
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("commit job timed out")
}

func (c *PodCleaner) recordDeploymentSuspendFailure(ctx context.Context, namespace, name, message string) error {
	deploy, err := c.k8sClient.GetDeployment(ctx, namespace, name)
	if err != nil {
		return err
	}
	deploy.Annotations = recordSuspendFailure(deploy.Annotations, message)
	_, err = c.k8sClient.GetClientset().AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	return err
}

func (c *PodCleaner) recordStatefulSetSuspendFailure(ctx context.Context, namespace, name, message string) error {
	sts, err := c.k8sClient.GetStatefulSet(ctx, namespace, name)
	if err != nil {
		return err
	}
	sts.Annotations = recordSuspendFailure(sts.Annotations, message)
	_, err = c.k8sClient.GetClientset().AppsV1().StatefulSets(namespace).Update(ctx, sts, metav1.UpdateOptions{})
	return err
}

func recordSuspendFailure(annotations map[string]string, message string) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["genet.io/suspended"] = "false"
	annotations["genet.io/suspend-message"] = message
	return annotations
}
