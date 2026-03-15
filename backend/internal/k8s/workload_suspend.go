package k8s

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func SelectRepresentativePod(pods []corev1.Pod) (*corev1.Pod, error) {
	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods available")
	}

	items := append([]corev1.Pod(nil), pods...)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	for i := range items {
		if items[i].Status.Phase == corev1.PodRunning {
			return &items[i], nil
		}
	}
	for i := range items {
		if !isTerminalPodPhase(items[i].Status.Phase) {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("no active pod available")
}

func isTerminalPodPhase(phase corev1.PodPhase) bool {
	return phase == corev1.PodSucceeded || phase == corev1.PodFailed
}

func applySuspendMetadata(annotations map[string]string, replicas int32, image, sourcePod string, suspendedAt time.Time) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["genet.io/suspend-enabled"] = "true"
	annotations["genet.io/suspended"] = "true"
	annotations["genet.io/suspended-image"] = image
	annotations["genet.io/suspended-replicas"] = strconv.FormatInt(int64(replicas), 10)
	annotations["genet.io/suspended-source-pod"] = sourcePod
	annotations["genet.io/suspended-at"] = suspendedAt.Format(time.RFC3339)
	annotations["genet.io/suspend-message"] = "suspended by scheduled cleanup"
	return annotations
}

func applyResumeMetadata(annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["genet.io/suspended"] = "false"
	annotations["genet.io/suspend-message"] = "resumed by user"
	return annotations
}

func (c *Client) SuspendDeployment(ctx context.Context, namespace, name, image, sourcePod string, suspendedAt time.Time) (*appsv1.Deployment, error) {
	deploy, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if len(deploy.Spec.Template.Spec.Containers) == 0 {
		return nil, fmt.Errorf("deployment 模板缺少容器")
	}

	replicas := int32(0)
	if deploy.Spec.Replicas != nil {
		replicas = *deploy.Spec.Replicas
	}
	deploy.Spec.Template.Spec.Containers[0].Image = image
	zero := int32(0)
	deploy.Spec.Replicas = &zero
	deploy.Annotations = applySuspendMetadata(deploy.Annotations, replicas, image, sourcePod, suspendedAt)

	updated, err := c.clientset.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("挂起 Deployment 失败: %w", err)
	}
	return updated, nil
}

func (c *Client) SuspendStatefulSet(ctx context.Context, namespace, name, image, sourcePod string, suspendedAt time.Time) (*appsv1.StatefulSet, error) {
	sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if len(sts.Spec.Template.Spec.Containers) == 0 {
		return nil, fmt.Errorf("statefulset 模板缺少容器")
	}

	replicas := int32(0)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}
	sts.Spec.Template.Spec.Containers[0].Image = image
	zero := int32(0)
	sts.Spec.Replicas = &zero
	sts.Annotations = applySuspendMetadata(sts.Annotations, replicas, image, sourcePod, suspendedAt)

	updated, err := c.clientset.AppsV1().StatefulSets(namespace).Update(ctx, sts, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("挂起 StatefulSet 失败: %w", err)
	}
	return updated, nil
}

func parseSuspendedReplicas(annotations map[string]string, resourceType string) (int32, error) {
	replicasText := strings.TrimSpace(annotations["genet.io/suspended-replicas"])
	if replicasText == "" {
		return 0, fmt.Errorf("%s 缺少挂起恢复元数据", resourceType)
	}
	replicas64, err := strconv.ParseInt(replicasText, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s 挂起副本数无效: %w", resourceType, err)
	}
	return int32(replicas64), nil
}
