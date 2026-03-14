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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type StatefulSetSpec struct {
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

func statefulSetServiceName(name string) string {
	return fmt.Sprintf("%s-headless", name)
}

func (c *Client) CreateStatefulSet(ctx context.Context, spec *StatefulSetSpec) (*appsv1.StatefulSet, error) {
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
		EnableNodeRank:         true,
		SharedNodeTotalDevices: spec.SharedNodeTotalDevices,
	})
	if err != nil {
		return nil, err
	}

	labels := map[string]string{
		"genet.io/user":          spec.Username,
		"genet.io/managed":       "true",
		"genet.io/workload-kind": "statefulset",
		"genet.io/workload-name": spec.Name,
		"app":                    spec.Name,
	}
	annotations := map[string]string{
		"genet.io/created-at":   time.Now().Format(time.RFC3339),
		"genet.io/email":        spec.Email,
		"genet.io/gpu-type":     spec.GPUType,
		"genet.io/gpu-count":    fmt.Sprintf("%d", spec.GPUCount),
		"genet.io/cpu":          spec.CPU,
		"genet.io/memory":       spec.Memory,
		"genet.io/shm-size":     spec.ShmSize,
		"genet.io/image":        spec.Image,
		"genet.io/replicas":     fmt.Sprintf("%d", spec.Replicas),
		"genet.io/service-name": statefulSetServiceName(spec.Name),
	}

	for _, vol := range c.config.Storage.GetEffectiveVolumes() {
		if vol.Type != "pvc" || strings.ToLower(vol.Scope) != "pod" {
			continue
		}
		templateName := SanitizeK8sName(vol.Name)
		for i := range runtimeSpec.Volumes {
			if runtimeSpec.Volumes[i].Name == templateName && runtimeSpec.Volumes[i].PersistentVolumeClaim != nil {
				runtimeSpec.Volumes[i].PersistentVolumeClaim.ClaimName = templateName
			}
		}
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetServiceName(spec.Name),
			Namespace: spec.Namespace,
			Labels:    cloneStringMap(labels),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector: map[string]string{
				"genet.io/workload-name": spec.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "placeholder",
					Port:       1,
					TargetPort: intstr.FromInt(1),
				},
			},
		},
	}
	if _, err := c.clientset.CoreV1().Services(spec.Namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("创建 StatefulSet 头服务失败: %w", err)
	}

	storageTypeAnnotation := ""
	storageVolumes := c.config.Storage.GetEffectiveVolumes()
	if len(storageVolumes) > 0 {
		storageTypeAnnotation = storageVolumes[0].Type
	}
	annotations["genet.io/storage-type"] = storageTypeAnnotation

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   spec.Namespace,
			Labels:      cloneStringMap(labels),
			Annotations: cloneStringMap(annotations),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            int32Ptr(spec.Replicas),
			ServiceName:         statefulSetServiceName(spec.Name),
			PodManagementPolicy: appsv1.ParallelPodManagement,
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
			VolumeClaimTemplates: c.buildStatefulSetVolumeClaimTemplates(spec.Username, spec.Name),
		},
	}

	c.log.Info("Creating statefulset",
		zap.String("name", spec.Name),
		zap.String("namespace", spec.Namespace),
		zap.Int32("replicas", spec.Replicas))

	created, err := c.clientset.AppsV1().StatefulSets(spec.Namespace).Create(ctx, sts, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("创建 StatefulSet 失败: %w", err)
	}
	return created, nil
}

func (c *Client) buildStatefulSetVolumeClaimTemplates(username, workloadName string) []corev1.PersistentVolumeClaim {
	storageVolumes := c.config.Storage.GetEffectiveVolumes()
	templates := make([]corev1.PersistentVolumeClaim, 0, len(storageVolumes))
	for _, vol := range storageVolumes {
		if vol.Type != "pvc" || strings.ToLower(vol.Scope) != "pod" {
			continue
		}
		size := vol.Size
		if size == "" {
			size = "50Gi"
		}
		accessMode := c.parseAccessMode(vol.AccessMode)
		template := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: SanitizeK8sName(vol.Name),
				Labels: map[string]string{
					"genet.io/managed":         "true",
					"genet.io/workload-kind":   "statefulset",
					"genet.io/workload-name":   workloadName,
					"genet.io/storage-scope":   "pod",
					"genet.io/user-identifier": username,
					"genet.io/volume-name":     vol.Name,
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
		if vol.StorageClass != "" {
			template.Spec.StorageClassName = &vol.StorageClass
		}
		templates = append(templates, template)
	}
	return templates
}

func (c *Client) GetStatefulSet(ctx context.Context, namespace, name string) (*appsv1.StatefulSet, error) {
	return c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) ListStatefulSets(ctx context.Context, namespace string) ([]appsv1.StatefulSet, error) {
	list, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "genet.io/managed=true,genet.io/workload-kind=statefulset",
	})
	if err != nil {
		return nil, err
	}
	items := append([]appsv1.StatefulSet(nil), list.Items...)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreationTimestamp.Time.After(items[j].CreationTimestamp.Time)
	})
	return items, nil
}

func (c *Client) ListStatefulSetPods(ctx context.Context, namespace, name string) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("genet.io/workload-name=%s", name),
	})
	if err != nil {
		return nil, err
	}
	items := append([]corev1.Pod(nil), list.Items...)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (c *Client) DeleteStatefulSet(ctx context.Context, namespace, name string) error {
	err := c.clientset.AppsV1().StatefulSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("删除 StatefulSet 失败: %w", err)
	}
	svcName := statefulSetServiceName(name)
	if err := c.clientset.CoreV1().Services(namespace).Delete(ctx, svcName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("删除 StatefulSet 头服务失败: %w", err)
	}
	return nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func int32Ptr(v int32) *int32 {
	return &v
}
