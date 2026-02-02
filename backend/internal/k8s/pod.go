package k8s

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
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
	Email      string // 用户邮箱
	Image      string
	GPUCount   int
	GPUType    string
	CPU        string // CPU 核数，如 "4"
	Memory     string // 内存大小，如 "8Gi"
	HTTPProxy  string // HTTP 代理
	HTTPSProxy string // HTTPS 代理
	NoProxy    string // 不代理列表
	// 高级配置
	NodeName   string // 指定调度节点（可选）
	GPUDevices []int  // 指定 GPU 卡编号（可选），如 [0, 2, 5]
}

// CreatePod 创建 Pod
func (c *Client) CreatePod(ctx context.Context, spec *PodSpec) (*corev1.Pod, error) {
	c.log.Info("Creating pod",
		zap.String("name", spec.Name),
		zap.String("namespace", spec.Namespace),
		zap.String("user", spec.Username),
		zap.String("image", spec.Image),
		zap.Int("gpuCount", spec.GPUCount),
		zap.String("gpuType", spec.GPUType),
		zap.String("nodeName", spec.NodeName),
		zap.Ints("gpuDevices", spec.GPUDevices))

	// 构建代理配置脚本片段
	proxySetupScript := ""
	if spec.HTTPProxy != "" || spec.HTTPSProxy != "" {
		c.log.Debug("Configuring proxy for pod",
			zap.String("httpProxy", spec.HTTPProxy),
			zap.String("httpsProxy", spec.HTTPSProxy))
		// 代理配置内容
		proxyConfig := fmt.Sprintf(`
# Proxy Configuration (by Genet)
export HTTP_PROXY="%s"
export HTTPS_PROXY="%s"
export http_proxy="%s"
export https_proxy="%s"
export NO_PROXY="%s"
export no_proxy="%s"
`, spec.HTTPProxy, spec.HTTPSProxy, spec.HTTPProxy, spec.HTTPSProxy, spec.NoProxy, spec.NoProxy)

		proxySetupScript = fmt.Sprintf(`
# 创建代理配置文件
cat > /etc/profile.d/proxy.sh << 'PROXYEOF'%s
PROXYEOF
chmod +x /etc/profile.d/proxy.sh 2>/dev/null || true

# 同时写入 ~/.profile (sh 登录 shell)
mkdir -p /root
cat >> /root/.profile << 'PROXYEOF'%s
PROXYEOF

# 同时写入 ~/.bashrc (bash 交互式 shell)
cat >> /root/.bashrc << 'PROXYEOF'%s
PROXYEOF

# 使当前 shell 生效
export HTTP_PROXY="%s"
export HTTPS_PROXY="%s"
export http_proxy="%s"
export https_proxy="%s"
export NO_PROXY="%s"
export no_proxy="%s"
echo "Proxy configured: HTTP_PROXY=%s, HTTPS_PROXY=%s"
`, proxyConfig, proxyConfig, proxyConfig,
			spec.HTTPProxy, spec.HTTPSProxy, spec.HTTPProxy, spec.HTTPSProxy, spec.NoProxy, spec.NoProxy,
			spec.HTTPProxy, spec.HTTPSProxy)
	}

	// 使用配置的启动脚本模板
	scriptTemplate := c.config.Pod.StartupScript
	if scriptTemplate == "" {
		c.log.Error("Pod startup script not configured")
		return nil, fmt.Errorf("pod.startupScript 未配置")
	}

	// 渲染启动脚本
	tmpl, err := template.New("startup").Parse(scriptTemplate)
	if err != nil {
		c.log.Error("Failed to parse startup script template", zap.Error(err))
		return nil, fmt.Errorf("解析启动脚本模板失败: %w", err)
	}

	var scriptBuf bytes.Buffer
	err = tmpl.Execute(&scriptBuf, map[string]interface{}{
		"ProxyScript": proxySetupScript,
	})
	if err != nil {
		c.log.Error("Failed to render startup script", zap.Error(err))
		return nil, fmt.Errorf("渲染启动脚本失败: %w", err)
	}

	startupScript := scriptBuf.String()

	// 主容器（VolumeMounts 在后面动态添加）
	container := corev1.Container{
		Name:         "workspace",
		Image:        spec.Image,
		Command:      []string{"/bin/sh", "-c"},
		Args:         []string{startupScript},
		VolumeMounts: []corev1.VolumeMount{},
	}

	// 应用资源配置（优先使用用户指定的值）
	cpuRequest := "2"
	memoryRequest := "4Gi"
	if spec.CPU != "" {
		cpuRequest = spec.CPU
	}
	if spec.Memory != "" {
		memoryRequest = spec.Memory
	}

	container.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(memoryRequest),
			corev1.ResourceCPU:    resource.MustParse(cpuRequest),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(memoryRequest),
			corev1.ResourceCPU:    resource.MustParse(cpuRequest),
		},
	}

	c.log.Debug("Pod resources configured",
		zap.String("cpu", cpuRequest),
		zap.String("memory", memoryRequest))

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

	// 应用 Lifecycle Hook（preStop/postStart）
	if c.config.Pod.Lifecycle != nil {
		container.Lifecycle = c.config.Pod.Lifecycle
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

	// 如果需要加速卡（GPU/NPU）
	var runtimeClassName *string
	if spec.GPUCount > 0 {
		c.log.Debug("Configuring accelerator resources",
			zap.Int("count", spec.GPUCount),
			zap.String("type", spec.GPUType),
			zap.String("schedulingMode", c.config.GPU.SchedulingMode))

		// 从配置中查找对应的资源名称和类型
		resourceName := "nvidia.com/gpu" // 默认 NVIDIA GPU
		acceleratorType := "nvidia"
		for _, gpuType := range c.config.GPU.AvailableTypes {
			if gpuType.Name == spec.GPUType {
				resourceName = gpuType.ResourceName
				if gpuType.Type != "" {
					acceleratorType = gpuType.Type
				}
				break
			}
		}

		// 判断调度模式
		isSharing := c.config.GPU.SchedulingMode == "sharing"

		if isSharing {
			// 共享模式：必须指定节点和 GPU 卡
			if spec.NodeName == "" || len(spec.GPUDevices) == 0 {
				return nil, fmt.Errorf("共享模式下必须指定节点和 GPU 卡")
			}

			// 检查共享上限
			if c.config.GPU.MaxPodsPerGPU > 0 {
				podsPerGPU := c.countPodsPerGPU(ctx, spec.NodeName)
				for _, deviceIdx := range spec.GPUDevices {
					if podsPerGPU[deviceIdx] >= c.config.GPU.MaxPodsPerGPU {
						return nil, fmt.Errorf("GPU 卡 %d 已达到共享上限 (%d 个 Pod)", deviceIdx, c.config.GPU.MaxPodsPerGPU)
					}
				}
			}

			// 共享模式：只设置环境变量，不请求 K8s GPU 资源
			deviceStr := intsToCommaString(spec.GPUDevices)
			switch acceleratorType {
			case "nvidia":
				container.Env = append(container.Env,
					corev1.EnvVar{Name: "NVIDIA_VISIBLE_DEVICES", Value: deviceStr},
					corev1.EnvVar{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "compute,utility"},
				)
			case "ascend":
				container.Env = append(container.Env,
					corev1.EnvVar{Name: "ASCEND_RT_VISIBLE_DEVICES", Value: deviceStr},
					corev1.EnvVar{Name: "ASCEND_GLOBAL_LOG_LEVEL", Value: "3"},
				)
			}

			// 设置 RuntimeClassName（如果配置了）
			if c.config.GPU.RuntimeClassName != "" {
				runtimeClassName = &c.config.GPU.RuntimeClassName
			}

			c.log.Debug("Accelerator configured in sharing mode",
				zap.String("acceleratorType", acceleratorType),
				zap.String("devices", deviceStr))
		} else {
			// 独占模式：请求 K8s GPU 资源
			container.Resources.Requests[corev1.ResourceName(resourceName)] = resource.MustParse(fmt.Sprintf("%d", spec.GPUCount))
			container.Resources.Limits[corev1.ResourceName(resourceName)] = resource.MustParse(fmt.Sprintf("%d", spec.GPUCount))

			// 根据加速卡类型设置环境变量
			switch acceleratorType {
			case "nvidia":
				// NVIDIA GPU: 设置 DRIVER_CAPABILITIES
				container.Env = append(container.Env,
					corev1.EnvVar{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "compute,utility"},
				)
				// 如果用户指定了具体的 GPU 卡，设置 NVIDIA_VISIBLE_DEVICES
				// 否则让 Device Plugin 自动注入
				if len(spec.GPUDevices) > 0 {
					deviceStr := intsToCommaString(spec.GPUDevices)
					container.Env = append(container.Env,
						corev1.EnvVar{Name: "NVIDIA_VISIBLE_DEVICES", Value: deviceStr},
					)
					c.log.Debug("Set NVIDIA_VISIBLE_DEVICES for specific GPU selection",
						zap.String("devices", deviceStr))
				}
			case "ascend":
				// 华为昇腾 NPU: 设置 ASCEND 相关环境变量
				container.Env = append(container.Env,
					corev1.EnvVar{Name: "ASCEND_GLOBAL_LOG_LEVEL", Value: "3"}, // 日志级别: 0-DEBUG, 1-INFO, 2-WARNING, 3-ERROR
				)
				// 如果用户指定了具体的 NPU 卡，设置 ASCEND_VISIBLE_DEVICES
				if len(spec.GPUDevices) > 0 {
					deviceStr := intsToCommaString(spec.GPUDevices)
					container.Env = append(container.Env,
						corev1.EnvVar{Name: "ASCEND_RT_VISIBLE_DEVICES", Value: deviceStr},
					)
					c.log.Debug("Set ASCEND_RT_VISIBLE_DEVICES for specific NPU selection",
						zap.String("devices", deviceStr))
				}
			}

			c.log.Debug("Accelerator configured in exclusive mode",
				zap.String("resourceName", resourceName),
				zap.String("acceleratorType", acceleratorType),
				zap.Int("count", spec.GPUCount))
		}
	}

	// 构建存储卷（支持多存储卷配置）
	storageVolumes := c.config.Storage.GetEffectiveVolumes()
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount
	var storageTypeAnnotation string

	for _, storageVol := range storageVolumes {
		volume, volumeMount := c.buildStorageVolume(storageVol, spec.Username)
		volumes = append(volumes, volume)
		volumeMounts = append(volumeMounts, volumeMount)

		c.log.Info("Storage volume configured",
			zap.String("name", storageVol.Name),
			zap.String("type", storageVol.Type),
			zap.String("mountPath", storageVol.MountPath),
			zap.String("user", spec.Username))
	}

	// 添加 VolumeMounts 到容器
	container.VolumeMounts = append(container.VolumeMounts, volumeMounts...)

	// 设置 annotation（使用第一个卷的类型作为主要存储类型）
	if len(storageVolumes) > 0 {
		storageTypeAnnotation = storageVolumes[0].Type
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
				"genet.io/created-at":   time.Now().Format(time.RFC3339),
				"genet.io/email":        spec.Email, // 用户邮箱
				"genet.io/gpu-type":     spec.GPUType,
				"genet.io/gpu-count":    fmt.Sprintf("%d", spec.GPUCount),
				"genet.io/cpu":          cpuRequest,
				"genet.io/memory":       memoryRequest,
				"genet.io/image":        spec.Image,
				"genet.io/storage-type": storageTypeAnnotation,
				"genet.io/gpu-devices":  intsToCommaString(spec.GPUDevices), // 记录指定的 GPU 卡编号
			},
		},
		Spec: corev1.PodSpec{
			AutomountServiceAccountToken: boolPtr(false),
			HostNetwork:                  c.config.Pod.HostNetwork,
			RestartPolicy:                corev1.RestartPolicyNever,
			Containers:                   []corev1.Container{container},
			Volumes:                      volumes,
		},
	}

	// 应用 RuntimeClassName（共享模式下可能需要）
	if runtimeClassName != nil {
		pod.Spec.RuntimeClassName = runtimeClassName
	}

	// 应用 DNS Policy
	if c.config.Pod.DNSPolicy != "" {
		pod.Spec.DNSPolicy = c.config.Pod.DNSPolicy
	}

	// 应用 DNS Config
	if c.config.Pod.DNSConfig != nil {
		pod.Spec.DNSConfig = c.config.Pod.DNSConfig
	}

	// 如果指定了节点名称，直接调度到该节点
	if spec.NodeName != "" {
		pod.Spec.NodeName = spec.NodeName
		c.log.Debug("Pod scheduled to specific node",
			zap.String("nodeName", spec.NodeName))
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
			// 清理卷名称，确保符合 K8s 命名规范
			sanitizedName := SanitizeK8sName(extraVol.Name)

			// 添加 VolumeMount 到容器
			pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
				Name:      sanitizedName,
				MountPath: extraVol.MountPath,
				ReadOnly:  extraVol.ReadOnly,
			})

			// 添加 Volume 到 Pod
			volume := corev1.Volume{Name: sanitizedName}
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
				c.log.Debug("Applied GPU node selector",
					zap.String("gpuType", spec.GPUType),
					zap.Any("nodeSelector", gpuType.NodeSelector))
				break
			}
		}
	}

	createdPod, err := c.clientset.CoreV1().Pods(spec.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		c.log.Error("Failed to create pod",
			zap.String("name", spec.Name),
			zap.String("namespace", spec.Namespace),
			zap.Error(err))
		return nil, err
	}

	c.log.Info("Pod created successfully",
		zap.String("name", createdPod.Name),
		zap.String("namespace", createdPod.Namespace),
		zap.String("uid", string(createdPod.UID)))

	return createdPod, nil
}

// countPodsPerGPU 统计指定节点上每张 GPU 卡被多少个 Pod 使用
func (c *Client) countPodsPerGPU(ctx context.Context, nodeName string) map[int]int {
	result := make(map[int]int)

	pods, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s,status.phase=Running", nodeName),
	})
	if err != nil {
		c.log.Warn("Failed to list pods for GPU counting",
			zap.String("nodeName", nodeName),
			zap.Error(err))
		return result
	}

	for _, pod := range pods.Items {
		if devices := pod.Annotations["genet.io/gpu-devices"]; devices != "" {
			for _, d := range parseGPUDevices(devices) {
				result[d]++
			}
		}
	}

	c.log.Debug("Counted pods per GPU",
		zap.String("nodeName", nodeName),
		zap.Any("podsPerGPU", result))

	return result
}

// parseGPUDevices 解析 GPU 设备列表字符串（如 "0,1,2"）
func parseGPUDevices(devices string) []int {
	if devices == "" {
		return nil
	}
	var result []int
	for _, s := range strings.Split(devices, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if idx, err := strconv.Atoi(s); err == nil {
			result = append(result, idx)
		}
	}
	return result
}

// DeletePod 删除 Pod
func (c *Client) DeletePod(ctx context.Context, namespace, name string) error {
	c.log.Info("Deleting pod",
		zap.String("name", name),
		zap.String("namespace", namespace))

	err := c.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		c.log.Error("Failed to delete pod",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return fmt.Errorf("删除 Pod 失败: %w", err)
	}

	if errors.IsNotFound(err) {
		c.log.Debug("Pod not found, already deleted",
			zap.String("name", name),
			zap.String("namespace", namespace))
	} else {
		c.log.Info("Pod deleted successfully",
			zap.String("name", name),
			zap.String("namespace", namespace))
	}

	return nil
}

// GetPod 获取 Pod
func (c *Client) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		c.log.Debug("Failed to get pod",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return nil, err
	}
	return pod, nil
}

// PodExists 检查 Pod 是否存在
func (c *Client) PodExists(ctx context.Context, namespace, name string) bool {
	_, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	return err == nil
}

// ListPods 列出用户的所有 Pod
func (c *Client) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "genet.io/managed=true",
	})
	if err != nil {
		c.log.Debug("Failed to list pods",
			zap.String("namespace", namespace),
			zap.Error(err))
		return nil, err
	}

	c.log.Debug("Listed pods",
		zap.String("namespace", namespace),
		zap.Int("count", len(list.Items)))

	return list.Items, nil
}

// GetPodLogs 获取 Pod 日志
func (c *Client) GetPodLogs(ctx context.Context, namespace, name string, tailLines int64) (string, error) {
	c.log.Debug("Getting pod logs",
		zap.String("name", name),
		zap.String("namespace", namespace),
		zap.Int64("tailLines", tailLines))

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(name, &corev1.PodLogOptions{
		TailLines: &tailLines,
	})

	logs, err := req.Do(ctx).Raw()
	if err != nil {
		c.log.Error("Failed to get pod logs",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return "", err
	}

	return string(logs), nil
}

// buildStorageVolume 根据存储卷配置构建 K8s Volume 和 VolumeMount
func (c *Client) buildStorageVolume(storageVol models.StorageVolume, username string) (corev1.Volume, corev1.VolumeMount) {
	// 清理卷名称，确保符合 K8s 命名规范（下划线转连字符等）
	sanitizedVolumeName := SanitizeK8sName(storageVol.Name)

	volumeMount := corev1.VolumeMount{
		Name:      sanitizedVolumeName,
		MountPath: storageVol.MountPath,
		ReadOnly:  storageVol.ReadOnly,
	}

	var volume corev1.Volume

	if storageVol.Type == "hostpath" {
		// HostPath 模式
		// 支持 {username} 变量替换
		hostPath := expandPathTemplate(storageVol.HostPath, username, storageVol.Name)
		hostPathType := corev1.HostPathDirectoryOrCreate
		volume = corev1.Volume{
			Name: sanitizedVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath,
					Type: &hostPathType,
				},
			},
		}
	} else {
		// PVC 模式（默认）
		pvcName := storageVol.PVCNameTemplate
		if pvcName == "" {
			// 默认 PVC 命名: genet-<username>-<volumeName>
			pvcName = fmt.Sprintf("genet-%s-%s", username, sanitizedVolumeName)
		} else {
			// 支持 {username}, {volumeName} 变量替换
			pvcName = expandPathTemplate(pvcName, username, storageVol.Name)
		}

		volume = corev1.Volume{
			Name: sanitizedVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
					ReadOnly:  storageVol.ReadOnly,
				},
			},
		}
	}

	return volume, volumeMount
}

// expandPathTemplate 展开路径模板中的变量
// 支持: {username}, {volumeName}
func expandPathTemplate(tmpl, username, volumeName string) string {
	if tmpl == "" {
		return tmpl
	}
	result := strings.ReplaceAll(tmpl, "{username}", username)
	result = strings.ReplaceAll(result, "{volumeName}", volumeName)
	return result
}

// intsToCommaString 将整数切片转换为逗号分隔的字符串
// 例如: [0, 2, 5] -> "0,2,5"
func intsToCommaString(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	strs := make([]string, len(nums))
	for i, n := range nums {
		strs[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(strs, ",")
}

// GetPVCName 获取存储卷对应的 PVC 名称
// storageVol: 存储卷配置
// username: 用户名
// 返回 PVC 名称，如果是 HostPath 类型则返回空字符串
func (c *Client) GetPVCName(storageVol models.StorageVolume, username string) string {
	if storageVol.Type == "hostpath" {
		return ""
	}
	pvcName := storageVol.PVCNameTemplate
	if pvcName == "" {
		// 默认 PVC 命名: genet-<username>-<volumeName>
		pvcName = fmt.Sprintf("genet-%s-%s", username, storageVol.Name)
	} else {
		// 支持 {username}, {volumeName} 变量替换
		pvcName = expandPathTemplate(pvcName, username, storageVol.Name)
	}
	return pvcName
}

// GetStorageVolumes 获取有效的存储卷配置（对外暴露）
func (c *Client) GetStorageVolumes() []models.StorageVolume {
	return c.config.Storage.GetEffectiveVolumes()
}
