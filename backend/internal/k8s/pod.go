package k8s

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"text/template"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodSpec Pod 创建规格
type PodSpec struct {
	Name       string
	Namespace  string
	Username   string
	Image      string
	GPUCount   int
	GPUType    string
	SSHPort    int32
	Password   string
	TTLHours   int
	ExpiresAt  time.Time
	HTTPProxy  string // HTTP 代理
	HTTPSProxy string // HTTPS 代理
	NoProxy    string // 不代理列表
}

// GenerateSSHPort 基于用户名生成固定的 SSH 端口 (2200-3199)
func GenerateSSHPort(username string) int32 {
	hasher := fnv.New32a()
	hasher.Write([]byte(username))
	return int32(2200 + (hasher.Sum32() % 1000))
}

// GeneratePodName 生成 Pod 名称
func GeneratePodName(username string) string {
	return fmt.Sprintf("pod-%s-%d", username, time.Now().Unix())
}

// CreatePod 创建 Pod
func (c *Client) CreatePod(ctx context.Context, spec *PodSpec) (*corev1.Pod, error) {
	// 构建代理配置脚本片段
	proxySetupScript := ""
	if spec.HTTPProxy != "" || spec.HTTPSProxy != "" {
		proxySetupScript = fmt.Sprintf(`
# 配置代理到 ~/.bashrc
cat >> /root/.bashrc << 'PROXYEOF'

# Proxy Configuration (by Genet)
export HTTP_PROXY="%s"
export HTTPS_PROXY="%s"
export http_proxy="%s"
export https_proxy="%s"
export NO_PROXY="%s"
export no_proxy="%s"
PROXYEOF

# 使当前 shell 生效
export HTTP_PROXY="%s"
export HTTPS_PROXY="%s"
export http_proxy="%s"
export https_proxy="%s"
export NO_PROXY="%s"
export no_proxy="%s"
echo "Proxy configured: HTTP_PROXY=%s, HTTPS_PROXY=%s"
`, spec.HTTPProxy, spec.HTTPSProxy, spec.HTTPProxy, spec.HTTPSProxy, spec.NoProxy, spec.NoProxy,
			spec.HTTPProxy, spec.HTTPSProxy, spec.HTTPProxy, spec.HTTPSProxy, spec.NoProxy, spec.NoProxy,
			spec.HTTPProxy, spec.HTTPSProxy)
	}

	// 使用配置的启动脚本模板
	scriptTemplate := c.config.Pod.StartupScript
	if scriptTemplate == "" {
		return nil, fmt.Errorf("pod.startupScript 未配置")
	}

	// 渲染启动脚本
	tmpl, err := template.New("startup").Parse(scriptTemplate)
	if err != nil {
		return nil, fmt.Errorf("解析启动脚本模板失败: %w", err)
	}

	var scriptBuf bytes.Buffer
	err = tmpl.Execute(&scriptBuf, map[string]interface{}{
		"SSHPort":     spec.SSHPort,
		"Password":    spec.Password,
		"ProxyScript": proxySetupScript,
	})
	if err != nil {
		return nil, fmt.Errorf("渲染启动脚本失败: %w", err)
	}

	startupScript := scriptBuf.String()

	// 主容器
	container := corev1.Container{
		Name:    "workspace",
		Image:   spec.Image,
		Command: []string{"/bin/bash", "-c"},
		Args:    []string{startupScript},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "workspace",
				MountPath: "/workspace",
			},
		},
	}

	// 应用资源配置（优先使用配置，否则使用默认值）
	if c.config.Pod.Resources != nil {
		container.Resources = *c.config.Pod.Resources
	} else {
		// 默认资源配置
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("4Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("16Gi"),
				corev1.ResourceCPU:    resource.MustParse("8"),
			},
		}
	}

	// 应用安全上下文（优先使用配置，否则使用默认值）
	if c.config.Pod.SecurityContext != nil {
		container.SecurityContext = c.config.Pod.SecurityContext
	} else {
		// 默认安全上下文
		container.SecurityContext = &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_ADMIN"},
			},
		}
	}

	// 添加代理环境变量
	if spec.HTTPProxy != "" {
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "HTTP_PROXY", Value: spec.HTTPProxy},
			corev1.EnvVar{Name: "http_proxy", Value: spec.HTTPProxy},
		)
	}
	if spec.HTTPSProxy != "" {
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "HTTPS_PROXY", Value: spec.HTTPSProxy},
			corev1.EnvVar{Name: "https_proxy", Value: spec.HTTPSProxy},
		)
	}
	if spec.NoProxy != "" {
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "NO_PROXY", Value: spec.NoProxy},
			corev1.EnvVar{Name: "no_proxy", Value: spec.NoProxy},
		)
	}

	// 如果需要 GPU
	if spec.GPUCount > 0 {
		container.Resources.Requests["nvidia.com/gpu"] = resource.MustParse(fmt.Sprintf("%d", spec.GPUCount))
		container.Resources.Limits["nvidia.com/gpu"] = resource.MustParse(fmt.Sprintf("%d", spec.GPUCount))
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "NVIDIA_VISIBLE_DEVICES", Value: "all"},
			corev1.EnvVar{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "compute,utility"},
		)
	}

	// 构建 Pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"genet.io/user":    spec.Username,
				"genet.io/managed": "true",
				"app":              spec.Name,
			},
			Annotations: map[string]string{
				"genet.io/created-at": time.Now().Format(time.RFC3339),
				"genet.io/expires-at": spec.ExpiresAt.Format(time.RFC3339),
				"genet.io/gpu-type":   spec.GPUType,
				"genet.io/gpu-count":  fmt.Sprintf("%d", spec.GPUCount),
				"genet.io/image":      spec.Image,
				"genet.io/ssh-port":   fmt.Sprintf("%d", spec.SSHPort),
				"genet.io/password":   spec.Password,
			},
		},
		Spec: corev1.PodSpec{
			HostNetwork:   c.config.Pod.HostNetwork,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers:    []corev1.Container{container},
			Volumes: []corev1.Volume{
				{
					Name: "workspace",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: fmt.Sprintf("%s-workspace", spec.Username),
						},
					},
				},
			},
		},
	}

	// 应用 DNS Policy
	if c.config.Pod.DNSPolicy != "" {
		pod.Spec.DNSPolicy = c.config.Pod.DNSPolicy
	}

	// 应用 DNS Config
	if c.config.Pod.DNSConfig != nil {
		pod.Spec.DNSConfig = c.config.Pod.DNSConfig
	}

	// 应用 NodeSelector（合并全局配置和 GPU 特定配置）
	if c.config.Pod.NodeSelector != nil {
		pod.Spec.NodeSelector = make(map[string]string)
		// 复制全局 NodeSelector
		for k, v := range c.config.Pod.NodeSelector {
			pod.Spec.NodeSelector[k] = v
		}
	}

	// 应用 Affinity
	if c.config.Pod.Affinity != nil {
		pod.Spec.Affinity = c.config.Pod.Affinity
	}

	// 添加额外的通用存储（优先使用新的 K8s 原生格式）
	if len(c.config.Pod.ExtraVolumes) > 0 || len(c.config.Pod.ExtraVolumeMounts) > 0 {
		// 使用 K8s 原生格式配置
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, c.config.Pod.ExtraVolumeMounts...)
		pod.Spec.Volumes = append(pod.Spec.Volumes, c.config.Pod.ExtraVolumes...)
	} else {
		// 向后兼容：使用旧的 Storage.ExtraVolumes 配置
		for _, extraVol := range c.config.Storage.ExtraVolumes {
			// 添加 VolumeMount 到容器
			pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
				Name:      extraVol.Name,
				MountPath: extraVol.MountPath,
				ReadOnly:  extraVol.ReadOnly,
			})

			// 添加 Volume 到 Pod
			volume := corev1.Volume{Name: extraVol.Name}
			if extraVol.PVC != "" {
				volume.VolumeSource = corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: extraVol.PVC,
						ReadOnly:  extraVol.ReadOnly,
					},
				}
			} else if extraVol.HostPath != "" {
				volume.VolumeSource = corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: extraVol.HostPath,
					},
				}
			} else if extraVol.NFS != nil {
				volume.VolumeSource = corev1.VolumeSource{
					NFS: &corev1.NFSVolumeSource{
						Server:   extraVol.NFS.Server,
						Path:     extraVol.NFS.Path,
						ReadOnly: extraVol.ReadOnly,
					},
				}
			}
			pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
		}
	}

	// 如果需要 GPU，合并 GPU 特定的 NodeSelector（GPU 配置优先）
	if spec.GPUCount > 0 && spec.GPUType != "" {
		// 查找 GPU 配置
		for _, gpuType := range c.config.GPU.AvailableTypes {
			if gpuType.Name == spec.GPUType {
				if pod.Spec.NodeSelector == nil {
					pod.Spec.NodeSelector = make(map[string]string)
				}
				// 合并 GPU NodeSelector，GPU 配置优先
				for k, v := range gpuType.NodeSelector {
					pod.Spec.NodeSelector[k] = v
				}
				break
			}
		}
	}

	return c.clientset.CoreV1().Pods(spec.Namespace).Create(ctx, pod, metav1.CreateOptions{})
}

// DeletePod 删除 Pod
func (c *Client) DeletePod(ctx context.Context, namespace, name string) error {
	err := c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("删除 Pod 失败: %w", err)
	}
	return nil
}

// GetPod 获取 Pod
func (c *Client) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	return c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

// ListPods 列出用户的所有 Pod
func (c *Client) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "genet.io/managed=true",
	})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// GetPodLogs 获取 Pod 日志
func (c *Client) GetPodLogs(ctx context.Context, namespace, name string, tailLines int64) (string, error) {
	req := c.clientset.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{
		TailLines: &tailLines,
	})

	logs, err := req.Do(ctx).Raw()
	if err != nil {
		return "", err
	}

	return string(logs), nil
}
