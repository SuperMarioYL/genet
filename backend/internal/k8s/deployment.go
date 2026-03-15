package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentSpec struct {
	Name       string
	Namespace  string
	Username   string
	Email      string
	Image      string
	GPUCount   int
	GPUType    string
	CPU        string
	Memory     string
	ShmSize    string
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
	NodeName   string
	Replicas   int32
	UserMounts []models.UserMount

	SharedNodeTotalDevices int
}

func (c *Client) CreateDeployment(ctx context.Context, spec *DeploymentSpec) (*appsv1.Deployment, error) {
	if spec.Replicas > 1 && c.hasPodScopedPVCVolumes() {
		return nil, fmt.Errorf("Deployment 多副本暂不支持 scope=pod PVC，请改用 StatefulSet 或调整存储配置")
	}

	runtimeSpec, err := c.buildWorkloadRuntime(ctx, &WorkloadRuntimeSpec{
		Name:                   spec.Name,
		Username:               spec.Username,
		Image:                  spec.Image,
		CPU:                    spec.CPU,
		Memory:                 spec.Memory,
		ShmSize:                spec.ShmSize,
		HTTPProxy:              spec.HTTPProxy,
		HTTPSProxy:             spec.HTTPSProxy,
		NoProxy:                spec.NoProxy,
		NodeName:               spec.NodeName,
		GPUCount:               spec.GPUCount,
		GPUType:                spec.GPUType,
		UserMounts:             spec.UserMounts,
		SharedNodeTotalDevices: spec.SharedNodeTotalDevices,
	})
	if err != nil {
		return nil, err
	}

	labels := map[string]string{
		"genet.io/user":          spec.Username,
		"genet.io/managed":       "true",
		"genet.io/workload-kind": "deployment",
		"genet.io/workload-name": spec.Name,
		"app":                    spec.Name,
	}
	annotations := map[string]string{
		"genet.io/created-at":      time.Now().Format(time.RFC3339),
		"genet.io/email":           spec.Email,
		"genet.io/gpu-type":        spec.GPUType,
		"genet.io/gpu-count":       fmt.Sprintf("%d", spec.GPUCount),
		"genet.io/cpu":             spec.CPU,
		"genet.io/memory":          spec.Memory,
		"genet.io/shm-size":        spec.ShmSize,
		"genet.io/image":           spec.Image,
		"genet.io/replicas":        fmt.Sprintf("%d", spec.Replicas),
		"genet.io/suspend-enabled": "true",
	}

	storageTypeAnnotation := ""
	storageVolumes := c.config.Storage.GetEffectiveVolumes()
	if len(storageVolumes) > 0 {
		storageTypeAnnotation = storageVolumes[0].Type
	}
	annotations["genet.io/storage-type"] = storageTypeAnnotation

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   spec.Namespace,
			Labels:      cloneStringMap(labels),
			Annotations: cloneStringMap(annotations),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(spec.Replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"genet.io/workload-name": spec.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      cloneStringMap(labels),
					Annotations: cloneStringMap(annotations),
				},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: boolPtr(false),
					HostNetwork:                  runtimeSpec.HostNetwork,
					RestartPolicy:                corev1.RestartPolicyAlways,
					Containers:                   []corev1.Container{runtimeSpec.Container},
					Volumes:                      runtimeSpec.Volumes,
					NodeSelector:                 runtimeSpec.NodeSelector,
					Affinity:                     runtimeSpec.Affinity,
					DNSPolicy:                    runtimeSpec.DNSPolicy,
					DNSConfig:                    runtimeSpec.DNSConfig,
					RuntimeClassName:             runtimeSpec.RuntimeClassName,
				},
			},
		},
	}

	c.log.Info("Creating deployment",
		zap.String("name", spec.Name),
		zap.String("namespace", spec.Namespace),
		zap.Int32("replicas", spec.Replicas))

	created, err := c.clientset.AppsV1().Deployments(spec.Namespace).Create(ctx, deploy, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("创建 Deployment 失败: %w", err)
	}
	return created, nil
}

func (c *Client) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	return c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) ListDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	list, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "genet.io/managed=true,genet.io/workload-kind=deployment",
	})
	if err != nil {
		return nil, err
	}
	items := append([]appsv1.Deployment(nil), list.Items...)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreationTimestamp.Time.After(items[j].CreationTimestamp.Time)
	})
	return items, nil
}

func (c *Client) ListDeploymentPods(ctx context.Context, namespace, name string) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("genet.io/workload-name=%s", name),
	})
	if err != nil {
		return nil, err
	}
	items := make([]corev1.Pod, 0, len(list.Items))
	for _, pod := range list.Items {
		if pod.Labels["genet.io/workload-kind"] != "deployment" {
			continue
		}
		items = append(items, pod)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (c *Client) DeleteDeployment(ctx context.Context, namespace, name string) error {
	err := c.clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("删除 Deployment 失败: %w", err)
	}
	return nil
}

func (c *Client) ResumeDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	deploy, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(deploy.Annotations["genet.io/suspended"], "true") {
		return nil, fmt.Errorf("deployment 未处于挂起状态")
	}
	image := strings.TrimSpace(deploy.Annotations["genet.io/suspended-image"])
	if image == "" {
		return nil, fmt.Errorf("deployment 缺少挂起恢复元数据")
	}
	replicas, err := parseSuspendedReplicas(deploy.Annotations, "deployment")
	if err != nil {
		return nil, err
	}
	if len(deploy.Spec.Template.Spec.Containers) == 0 {
		return nil, fmt.Errorf("deployment 模板缺少容器")
	}

	deploy.Spec.Template.Spec.Containers[0].Image = image
	deploy.Spec.Replicas = &replicas
	deploy.Annotations = applyResumeMetadata(deploy.Annotations)

	updated, err := c.clientset.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("恢复 Deployment 失败: %w", err)
	}
	return updated, nil
}

func (c *Client) HasPodScopedPVCVolumes() bool {
	return c.hasPodScopedPVCVolumes()
}

func (c *Client) hasPodScopedPVCVolumes() bool {
	for _, vol := range c.config.Storage.GetEffectiveVolumes() {
		if vol.Type == "pvc" && strings.ToLower(vol.Scope) == "pod" {
			return true
		}
	}
	return false
}
