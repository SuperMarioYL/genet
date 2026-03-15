package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	PoolType   string // 用户所属节点池 shared | exclusive
	Image      string
	GPUCount   int
	GPUType    string
	CPU        string // CPU 核数，如 "4"
	Memory     string // 内存大小，如 "8Gi"
	ShmSize    string // 共享内存大小（挂载到 /dev/shm），如 "1Gi"
	HTTPProxy  string // HTTP 代理
	HTTPSProxy string // HTTPS 代理
	NoProxy    string // 不代理列表
	// 高级配置
	NodeName   string             // 指定调度节点（可选）
	GPUDevices []int              // 指定 GPU 卡编号（可选），如 [0, 2, 5]
	UserMounts []models.UserMount // 用户自定义挂载（可选）
}

type PodLogOptions struct {
	TailLines  int64
	Previous   bool
	Follow     bool
	Timestamps bool
	SinceTime  *time.Time
}

func buildPodLogOptions(options PodLogOptions) *corev1.PodLogOptions {
	podLogOptions := &corev1.PodLogOptions{
		Previous:   options.Previous,
		Follow:     options.Follow,
		Timestamps: options.Timestamps,
	}

	if options.TailLines > 0 {
		tailLines := options.TailLines
		podLogOptions.TailLines = &tailLines
	}
	if options.SinceTime != nil {
		sinceTime := metav1.NewTime(options.SinceTime.UTC())
		podLogOptions.SinceTime = &sinceTime
	}

	return podLogOptions
}

func buildAutoInjectedDownwardEnvVars(containerName string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "NODE_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.hostIP"},
			},
		},
		{
			Name: "HOST_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.hostIP"},
			},
		},
		{
			Name: "NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"},
			},
		},
		{
			Name: "POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"},
			},
		},
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			},
		},
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
			},
		},
		{
			Name: "POD_SERVICE_ACCOUNT",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.serviceAccountName"},
			},
		},
		{
			Name: "POD_UID",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"},
			},
		},
		{
			Name: "CPU_REQUEST",
			ValueFrom: &corev1.EnvVarSource{
				ResourceFieldRef: &corev1.ResourceFieldSelector{
					ContainerName: containerName,
					Resource:      "requests.cpu",
				},
			},
		},
		{
			Name: "CPU_LIMIT",
			ValueFrom: &corev1.EnvVarSource{
				ResourceFieldRef: &corev1.ResourceFieldSelector{
					ContainerName: containerName,
					Resource:      "limits.cpu",
				},
			},
		},
		{
			Name: "MEMORY_REQUEST",
			ValueFrom: &corev1.EnvVarSource{
				ResourceFieldRef: &corev1.ResourceFieldSelector{
					ContainerName: containerName,
					Resource:      "requests.memory",
				},
			},
		},
		{
			Name: "MEMORY_LIMIT",
			ValueFrom: &corev1.EnvVarSource{
				ResourceFieldRef: &corev1.ResourceFieldSelector{
					ContainerName: containerName,
					Resource:      "limits.memory",
				},
			},
		},
	}
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
		"ProxyScript":      proxySetupScript,
		"CodeServerScript": buildCodeServerStartupScript(c.config.Pod.CodeServer),
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
		privileged := true
		container.SecurityContext = &corev1.SecurityContext{
			Privileged: &privileged,
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

	// 注入 Downward API 元数据与资源配额环境变量
	container.Env = append(container.Env, buildAutoInjectedDownwardEnvVars(container.Name)...)

	// 如果需要加速卡（GPU/NPU）
	var runtimeClassName *string
	acceleratorType := ""
	if spec.GPUCount > 0 {
		c.log.Debug("Configuring accelerator resources",
			zap.Int("count", spec.GPUCount),
			zap.String("type", spec.GPUType),
			zap.String("schedulingMode", c.config.GPU.SchedulingMode))

		// 从配置中查找对应的资源名称和类型
		resourceName := "nvidia.com/gpu" // 默认 NVIDIA GPU
		acceleratorType = "nvidia"
		for _, gpuType := range c.config.GPU.AvailableTypes {
			if gpuType.Name == spec.GPUType {
				resourceName = gpuType.ResourceName
				acceleratorType = inferAcceleratorType(gpuType.Type, resourceName)
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
					corev1.EnvVar{Name: "ASCEND_VISIBLE_DEVICES", Value: deviceStr},
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
			// 独占模式：只有 resourceName 非空时才请求 K8s GPU 资源
			if resourceName != "" {
				container.Resources.Requests[corev1.ResourceName(resourceName)] = resource.MustParse(fmt.Sprintf("%d", spec.GPUCount))
				container.Resources.Limits[corev1.ResourceName(resourceName)] = resource.MustParse(fmt.Sprintf("%d", spec.GPUCount))
			}

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
				// 如果用户指定了具体的 NPU 卡，同时设置 ASCEND_RT_VISIBLE_DEVICES 和 ASCEND_VISIBLE_DEVICES
				if len(spec.GPUDevices) > 0 {
					deviceStr := intsToCommaString(spec.GPUDevices)
					container.Env = append(container.Env,
						corev1.EnvVar{Name: "ASCEND_RT_VISIBLE_DEVICES", Value: deviceStr},
						corev1.EnvVar{Name: "ASCEND_VISIBLE_DEVICES", Value: deviceStr},
					)
					c.log.Debug("Set ASCEND_RT_VISIBLE_DEVICES and ASCEND_VISIBLE_DEVICES for specific NPU selection",
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
	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount
	var storageTypeAnnotation string

	// 配置共享内存卷（/dev/shm）
	if spec.ShmSize != "" {
		shmVolume, shmMount, err := buildShmVolume(spec.ShmSize)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, shmVolume)
		volumeMounts = append(volumeMounts, shmMount)
	}

	storageVolumes := c.config.Storage.GetEffectiveVolumes()
	for _, storageVol := range storageVolumes {
		volume, volumeMount := c.buildStorageVolume(storageVol, spec.Username, spec.Name)
		volumes = append(volumes, volume)
		volumeMounts = append(volumeMounts, volumeMount)

		c.log.Info("Storage volume configured",
			zap.String("name", storageVol.Name),
			zap.String("type", storageVol.Type),
			zap.String("mountPath", storageVol.MountPath),
			zap.String("user", spec.Username))
	}

	// 添加用户自定义挂载
	for i, userMount := range spec.UserMounts {
		volName := fmt.Sprintf("user-mount-%d", i)
		hostPathType := corev1.HostPathDirectoryOrCreate

		volumes = append(volumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: userMount.HostPath,
					Type: &hostPathType,
				},
			},
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: userMount.MountPath,
			ReadOnly:  userMount.ReadOnly,
		})

		c.log.Info("User mount configured",
			zap.String("hostPath", userMount.HostPath),
			zap.String("mountPath", userMount.MountPath),
			zap.Bool("readOnly", userMount.ReadOnly),
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
				"genet.io/shm-size":     spec.ShmSize,
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

	// 应用 NodeSelector（合并全局配置和 GPU 特定配置）
	if c.config.Pod.NodeSelector != nil {
		pod.Spec.NodeSelector = make(map[string]string)
		// 复制全局 NodeSelector
		for k, v := range c.config.Pod.NodeSelector {
			pod.Spec.NodeSelector[k] = v
		}
	}

	// 构建 Affinity：
	// 1. 深拷贝全局配置，避免跨请求污染
	// 2. 当指定节点时，将 hostname 约束按 AND 合并到每个 required term，避免 term 追加导致 OR 放宽
	pod.Spec.Affinity = buildPodAffinity(c.config.Pod.Affinity, spec.NodeName)
	applyUserPoolSchedulingConstraints(&pod.Spec, spec.PoolType, c.config)
	// 使用 nodeAffinity（而非 PodSpec.NodeName）可保留调度器参与，
	// WaitForFirstConsumer 的 PVC 仍可获得 selected-node annotation。
	if spec.NodeName != "" {
		c.log.Debug("Pod scheduled to specific node via nodeAffinity",
			zap.String("nodeName", spec.NodeName))
	}

	// 添加额外的通用存储（K8s 原生格式）
	if len(c.config.Pod.ExtraVolumes) > 0 || len(c.config.Pod.ExtraVolumeMounts) > 0 {
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, c.config.Pod.ExtraVolumeMounts...)
		pod.Spec.Volumes = append(pod.Spec.Volumes, c.config.Pod.ExtraVolumes...)
	}

	// Ascend 场景自动补充运行时工具目录挂载（不依赖 values 配置）
	if acceleratorType == "ascend" {
		ensureAscendHostMounts(&pod.Spec.Containers[0], &pod.Spec.Volumes, c.log)
	}

	// 合并 GPU/Platform 特定的 NodeSelector（配置优先）
	// 支持 CPU Only 模式：即使 GPUCount 为 0，也需要应用 NodeSelector（如 platform 选择）
	if spec.GPUType != "" {
		// 查找配置（可能是 GPU 类型或 CPU Only）
		for _, gpuType := range c.config.GPU.AvailableTypes {
			if gpuType.Name == spec.GPUType {
				if len(gpuType.NodeSelector) > 0 {
					if pod.Spec.NodeSelector == nil {
						pod.Spec.NodeSelector = make(map[string]string)
					}
					// 合并 NodeSelector，配置优先
					for k, v := range gpuType.NodeSelector {
						pod.Spec.NodeSelector[k] = v
					}
					c.log.Debug("Applied node selector from GPU/Platform config",
						zap.String("gpuType", spec.GPUType),
						zap.Any("nodeSelector", gpuType.NodeSelector))
				}
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
	allPods, err := c.ListAllPods(ctx, namespace)
	if err != nil {
		return nil, err
	}

	pods := make([]corev1.Pod, 0, len(allPods))
	for _, pod := range allPods {
		if isManagedWorkloadChildPod(&pod) {
			continue
		}
		pods = append(pods, pod)
	}

	c.log.Debug("Listed standalone pods",
		zap.String("namespace", namespace),
		zap.Int("count", len(pods)))

	return pods, nil
}

func (c *Client) ListAllPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	// 兼容旧逻辑：优先查询 Genet 管理的 Pod
	managedList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "genet.io/managed=true",
	})
	if err != nil {
		c.log.Debug("Failed to list pods",
			zap.String("namespace", namespace),
			zap.String("selector", "genet.io/managed=true"),
			zap.Error(err))
		return nil, err
	}

	// 新逻辑：同时纳管该 namespace 下的所有 Pod
	allList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.log.Debug("Failed to list pods",
			zap.String("namespace", namespace),
			zap.String("selector", "<all>"),
			zap.Error(err))
		return nil, err
	}

	pods := make([]corev1.Pod, 0, len(allList.Items))
	seen := make(map[string]struct{}, len(allList.Items))

	addPod := func(pod corev1.Pod) {
		key := string(pod.UID)
		if key == "" {
			key = fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		pods = append(pods, pod)
	}

	for _, pod := range managedList.Items {
		addPod(pod)
	}
	for _, pod := range allList.Items {
		addPod(pod)
	}

	c.log.Debug("Listed pods (merged managed and all namespace pods)",
		zap.String("namespace", namespace),
		zap.Int("managedCount", len(managedList.Items)),
		zap.Int("allCount", len(allList.Items)),
		zap.Int("mergedCount", len(pods)))

	return pods, nil
}

func isManagedWorkloadChildPod(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	switch pod.Labels["genet.io/workload-kind"] {
	case "statefulset", "deployment":
		return true
	}
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "StatefulSet" || owner.Kind == "Deployment" || owner.Kind == "ReplicaSet" {
			return true
		}
	}
	return false
}

// GetPodLogs 获取 Pod 日志
func (c *Client) GetPodLogs(ctx context.Context, namespace, name string, options PodLogOptions) (string, error) {
	c.log.Debug("Getting pod logs",
		zap.String("name", name),
		zap.String("namespace", namespace),
		zap.Int64("tailLines", options.TailLines),
		zap.Bool("previous", options.Previous),
		zap.Bool("follow", options.Follow),
		zap.Bool("timestamps", options.Timestamps))

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(name, buildPodLogOptions(options))

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

// StreamPodLogs 获取 Pod 日志流
func (c *Client) StreamPodLogs(ctx context.Context, namespace, name string, options PodLogOptions) (io.ReadCloser, error) {
	c.log.Debug("Streaming pod logs",
		zap.String("name", name),
		zap.String("namespace", namespace),
		zap.Int64("tailLines", options.TailLines),
		zap.Bool("previous", options.Previous),
		zap.Bool("follow", options.Follow),
		zap.Bool("timestamps", options.Timestamps))

	req := c.clientset.CoreV1().Pods(namespace).GetLogs(name, buildPodLogOptions(options))
	stream, err := req.Stream(ctx)
	if err != nil {
		c.log.Error("Failed to stream pod logs",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return nil, err
	}

	return stream, nil
}

// buildShmVolume 构建 /dev/shm 共享内存卷
func buildShmVolume(shmSize string) (corev1.Volume, corev1.VolumeMount, error) {
	qty, err := resource.ParseQuantity(shmSize)
	if err != nil {
		return corev1.Volume{}, corev1.VolumeMount{}, fmt.Errorf("无效共享内存大小 %q: %w", shmSize, err)
	}

	volume := corev1.Volume{
		Name: "genet-shm",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium:    corev1.StorageMediumMemory,
				SizeLimit: &qty,
			},
		},
	}

	volumeMount := corev1.VolumeMount{
		Name:      "genet-shm",
		MountPath: "/dev/shm",
	}

	return volume, volumeMount, nil
}

// buildStorageVolume 根据存储卷配置构建 K8s Volume 和 VolumeMount
func (c *Client) buildStorageVolume(storageVol models.StorageVolume, username, podName string) (corev1.Volume, corev1.VolumeMount) {
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
		// PVC 模式（默认）- 使用 GetPVCName 统一命名逻辑
		pvcName := c.GetPVCName(storageVol, username, podName)

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
// 支持: {username}, {volumeName}, {podName}
func expandPathTemplate(tmpl, username, volumeName string) string {
	if tmpl == "" {
		return tmpl
	}
	result := strings.ReplaceAll(tmpl, "{username}", username)
	result = strings.ReplaceAll(result, "{volumeName}", volumeName)
	return result
}

// expandPathTemplateWithPod 展开路径模板中的变量（包含 podName）
// 支持: {username}, {volumeName}, {podName}
func expandPathTemplateWithPod(tmpl, username, volumeName, podName string) string {
	if tmpl == "" {
		return tmpl
	}
	result := strings.ReplaceAll(tmpl, "{username}", username)
	result = strings.ReplaceAll(result, "{volumeName}", volumeName)
	result = strings.ReplaceAll(result, "{podName}", podName)
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

type autoHostMount struct {
	BaseName string
	Path     string
}

var ascendAutoHostMounts = []autoHostMount{
	{BaseName: "ascend-sbin", Path: "/usr/local/sbin"},
	{BaseName: "ascend-dcmi", Path: "/usr/local/dcmi"},
	{BaseName: "ascend-driver", Path: "/usr/local/Ascend/driver"},
}

func inferAcceleratorType(configType, resourceName string) string {
	if strings.TrimSpace(configType) != "" {
		return strings.ToLower(strings.TrimSpace(configType))
	}

	lowerResource := strings.ToLower(strings.TrimSpace(resourceName))
	switch {
	case lowerResource == "nvidia.com/gpu":
		return "nvidia"
	case strings.Contains(lowerResource, "ascend"),
		strings.Contains(lowerResource, "huawei"),
		strings.Contains(lowerResource, "npu"):
		return "ascend"
	default:
		return lowerResource
	}
}

func ensureAscendHostMounts(container *corev1.Container, volumes *[]corev1.Volume, log *zap.Logger) {
	for _, mount := range ascendAutoHostMounts {
		if hasVolumeMountPath(container.VolumeMounts, mount.Path) {
			continue
		}

		volumeName := findHostPathVolumeName(*volumes, mount.Path)
		if volumeName == "" {
			volumeName = uniqueVolumeName(*volumes, mount.BaseName)
			hostPathType := corev1.HostPathDirectory
			*volumes = append(*volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: mount.Path,
						Type: &hostPathType,
					},
				},
			})
		}

		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: mount.Path,
			ReadOnly:  true,
		})

		log.Info("Auto-mounted Ascend host path",
			zap.String("path", mount.Path),
			zap.String("volumeName", volumeName))
	}
}

func hasVolumeMountPath(volumeMounts []corev1.VolumeMount, mountPath string) bool {
	for _, vm := range volumeMounts {
		if vm.MountPath == mountPath {
			return true
		}
	}
	return false
}

func findHostPathVolumeName(volumes []corev1.Volume, hostPath string) string {
	for _, v := range volumes {
		if v.HostPath != nil && v.HostPath.Path == hostPath {
			return v.Name
		}
	}
	return ""
}

func uniqueVolumeName(volumes []corev1.Volume, base string) string {
	name := base
	for i := 0; ; i++ {
		exists := false
		for _, v := range volumes {
			if v.Name == name {
				exists = true
				break
			}
		}
		if !exists {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i+1)
	}
}

func buildPodAffinity(base *corev1.Affinity, nodeName string) *corev1.Affinity {
	if base == nil && nodeName == "" {
		return nil
	}

	var affinity *corev1.Affinity
	if base != nil {
		affinity = base.DeepCopy()
	} else {
		affinity = &corev1.Affinity{}
	}

	if nodeName != "" {
		enforceNodeNameAffinity(affinity, nodeName)
	}

	return affinity
}

func enforceNodeNameAffinity(affinity *corev1.Affinity, nodeName string) {
	if affinity.NodeAffinity == nil {
		affinity.NodeAffinity = &corev1.NodeAffinity{}
	}

	required := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	hostnameRequirement := corev1.NodeSelectorRequirement{
		Key:      "kubernetes.io/hostname",
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{nodeName},
	}

	if required == nil || len(required.NodeSelectorTerms) == 0 {
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{hostnameRequirement},
				},
			},
		}
		return
	}

	mergedTerms := make([]corev1.NodeSelectorTerm, 0, len(required.NodeSelectorTerms))
	for _, term := range required.NodeSelectorTerms {
		termCopy := corev1.NodeSelectorTerm{
			MatchExpressions: append([]corev1.NodeSelectorRequirement{}, term.MatchExpressions...),
			MatchFields:      append([]corev1.NodeSelectorRequirement{}, term.MatchFields...),
		}
		termCopy.MatchExpressions = append(termCopy.MatchExpressions, hostnameRequirement)
		mergedTerms = append(mergedTerms, termCopy)
	}
	required.NodeSelectorTerms = mergedTerms
}

// GetPVCName 获取存储卷对应的 PVC 名称
// storageVol: 存储卷配置
// username: 用户标识（userIdentifier）
// podName: Pod 名称（scope="pod" 时需要）
// 返回 PVC 名称，如果是 HostPath 类型则返回空字符串
func (c *Client) GetPVCName(storageVol models.StorageVolume, username, podName string) string {
	if storageVol.Type == "hostpath" {
		return ""
	}

	sanitizedVolumeName := SanitizeK8sName(storageVol.Name)
	scope := strings.ToLower(storageVol.Scope)
	pvcName := storageVol.PVCNameTemplate

	if scope == "pod" {
		// Pod 级作用域：每个 Pod 独立 PVC
		if pvcName == "" {
			pvcName = fmt.Sprintf("genet-%s-%s-%s", username, podName, sanitizedVolumeName)
		} else {
			pvcName = expandPathTemplateWithPod(pvcName, username, storageVol.Name, podName)
		}
	} else {
		// 用户级作用域（默认）：同一用户共享 PVC
		if pvcName == "" {
			pvcName = fmt.Sprintf("genet-%s-%s", username, sanitizedVolumeName)
		} else {
			pvcName = expandPathTemplate(pvcName, username, storageVol.Name)
		}
	}

	return pvcName
}

// GetStorageVolumes 获取有效的存储卷配置（对外暴露）
func (c *Client) GetStorageVolumes() []models.StorageVolume {
	return c.config.Storage.GetEffectiveVolumes()
}
