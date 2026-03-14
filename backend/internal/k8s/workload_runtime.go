package k8s

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type WorkloadRuntimeSpec struct {
	Name       string
	Username   string
	Image      string
	CPU        string
	Memory     string
	ShmSize    string
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
	Command    []string
	Args       []string
	WorkingDir string
	Env        []corev1.EnvVar

	NodeName   string
	GPUCount   int
	GPUType    string
	GPUDevices []int
	UserMounts []models.UserMount

	EnableNodeRank         bool
	SharedNodeTotalDevices int
}

type WorkloadRuntime struct {
	Container        corev1.Container
	Volumes          []corev1.Volume
	RuntimeClassName *string
	NodeSelector     map[string]string
	Affinity         *corev1.Affinity
	HostNetwork      bool
	DNSPolicy        corev1.DNSPolicy
	DNSConfig        *corev1.PodDNSConfig
}

func (c *Client) buildWorkloadRuntime(ctx context.Context, spec *WorkloadRuntimeSpec) (*WorkloadRuntime, error) {
	proxySetupScript := ""
	if spec.HTTPProxy != "" || spec.HTTPSProxy != "" {
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

	command := spec.Command
	args := spec.Args
	if len(command) == 0 {
		scriptTemplate := c.config.Pod.StartupScript
		if scriptTemplate == "" {
			return nil, fmt.Errorf("pod.startupScript 未配置")
		}

		tmpl, err := template.New("startup").Parse(scriptTemplate)
		if err != nil {
			return nil, fmt.Errorf("解析启动脚本模板失败: %w", err)
		}

		var scriptBuf bytes.Buffer
		if err := tmpl.Execute(&scriptBuf, map[string]interface{}{
			"ProxyScript":      proxySetupScript,
			"CodeServerScript": buildCodeServerStartupScript(c.config.Pod.CodeServer),
		}); err != nil {
			return nil, fmt.Errorf("渲染启动脚本失败: %w", err)
		}

		startupScript := scriptBuf.String()
		if spec.EnableNodeRank {
			startupScript = wrapStartupScriptWithNodeRank(startupScript)
		}

		command = []string{"/bin/sh", "-c"}
		args = []string{startupScript}
	}

	container := corev1.Container{
		Name:         "workspace",
		Image:        spec.Image,
		Command:      command,
		Args:         args,
		WorkingDir:   spec.WorkingDir,
		Env:          append([]corev1.EnvVar{}, spec.Env...),
		VolumeMounts: []corev1.VolumeMount{},
	}

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

	if c.config.Pod.SecurityContext != nil {
		container.SecurityContext = c.config.Pod.SecurityContext
	} else {
		privileged := true
		container.SecurityContext = &corev1.SecurityContext{
			Privileged: &privileged,
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_ADMIN"},
			},
		}
	}

	if c.config.Pod.Lifecycle != nil {
		container.Lifecycle = c.config.Pod.Lifecycle
	}

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

	container.Env = append(container.Env, buildAutoInjectedDownwardEnvVars(container.Name)...)
	if spec.EnableNodeRank {
		container.Env = append(container.Env, corev1.EnvVar{Name: "NODE_RANK", Value: ""})
	}

	var runtimeClassName *string
	acceleratorType := ""
	if spec.GPUCount > 0 {
		resourceName := "nvidia.com/gpu"
		acceleratorType = "nvidia"
		for _, gpuType := range c.config.GPU.AvailableTypes {
			if gpuType.Name == spec.GPUType {
				resourceName = gpuType.ResourceName
				acceleratorType = inferAcceleratorType(gpuType.Type, resourceName)
				break
			}
		}

		if c.config.GPU.SchedulingMode == "sharing" {
			if spec.NodeName == "" {
				return nil, fmt.Errorf("共享模式下必须指定节点")
			}
			if len(spec.GPUDevices) > 0 {
				if c.config.GPU.MaxPodsPerGPU > 0 {
					podsPerGPU := c.countPodsPerGPU(ctx, spec.NodeName)
					for _, deviceIdx := range spec.GPUDevices {
						if podsPerGPU[deviceIdx] >= c.config.GPU.MaxPodsPerGPU {
							return nil, fmt.Errorf("GPU 卡 %d 已达到共享上限 (%d 个 Pod)", deviceIdx, c.config.GPU.MaxPodsPerGPU)
						}
					}
				}

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
			} else if spec.EnableNodeRank && spec.SharedNodeTotalDevices > 0 {
				switch acceleratorType {
				case "nvidia":
					container.Env = append(container.Env,
						corev1.EnvVar{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "compute,utility"},
					)
				case "ascend":
					container.Env = append(container.Env,
						corev1.EnvVar{Name: "ASCEND_GLOBAL_LOG_LEVEL", Value: "3"},
					)
				}
				if len(container.Args) == 1 {
					container.Args[0] = wrapStartupScriptWithSharedDeviceAssignment(
						container.Args[0],
						acceleratorType,
						spec.GPUCount,
						spec.SharedNodeTotalDevices,
					)
				}
			} else {
				return nil, fmt.Errorf("共享模式下必须指定 GPU 卡或启用基于副本序号的自动分配")
			}
			if c.config.GPU.RuntimeClassName != "" {
				runtimeClassName = &c.config.GPU.RuntimeClassName
			}
		} else {
			if resourceName != "" {
				container.Resources.Requests[corev1.ResourceName(resourceName)] = resource.MustParse(fmt.Sprintf("%d", spec.GPUCount))
				container.Resources.Limits[corev1.ResourceName(resourceName)] = resource.MustParse(fmt.Sprintf("%d", spec.GPUCount))
			}

			switch acceleratorType {
			case "nvidia":
				container.Env = append(container.Env,
					corev1.EnvVar{Name: "NVIDIA_DRIVER_CAPABILITIES", Value: "compute,utility"},
				)
				if len(spec.GPUDevices) > 0 {
					container.Env = append(container.Env,
						corev1.EnvVar{Name: "NVIDIA_VISIBLE_DEVICES", Value: intsToCommaString(spec.GPUDevices)},
					)
				}
			case "ascend":
				container.Env = append(container.Env,
					corev1.EnvVar{Name: "ASCEND_GLOBAL_LOG_LEVEL", Value: "3"},
				)
				if len(spec.GPUDevices) > 0 {
					deviceStr := intsToCommaString(spec.GPUDevices)
					container.Env = append(container.Env,
						corev1.EnvVar{Name: "ASCEND_RT_VISIBLE_DEVICES", Value: deviceStr},
						corev1.EnvVar{Name: "ASCEND_VISIBLE_DEVICES", Value: deviceStr},
					)
				}
			}
		}
	}

	var volumes []corev1.Volume
	var volumeMounts []corev1.VolumeMount
	if spec.ShmSize != "" {
		shmVolume, shmMount, err := buildShmVolume(spec.ShmSize)
		if err != nil {
			return nil, err
		}
		volumes = append(volumes, shmVolume)
		volumeMounts = append(volumeMounts, shmMount)
	}

	for _, storageVol := range c.config.Storage.GetEffectiveVolumes() {
		volume, volumeMount := c.buildStorageVolume(storageVol, spec.Username, spec.Name)
		volumes = append(volumes, volume)
		volumeMounts = append(volumeMounts, volumeMount)
	}

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
	}

	container.VolumeMounts = append(container.VolumeMounts, volumeMounts...)
	if len(c.config.Pod.ExtraVolumeMounts) > 0 {
		container.VolumeMounts = append(container.VolumeMounts, c.config.Pod.ExtraVolumeMounts...)
	}
	if len(c.config.Pod.ExtraVolumes) > 0 {
		volumes = append(volumes, c.config.Pod.ExtraVolumes...)
	}
	if acceleratorType == "ascend" {
		ensureAscendHostMounts(&container, &volumes, c.log)
	}

	var nodeSelector map[string]string
	if c.config.Pod.NodeSelector != nil {
		nodeSelector = make(map[string]string, len(c.config.Pod.NodeSelector))
		for k, v := range c.config.Pod.NodeSelector {
			nodeSelector[k] = v
		}
	}
	if spec.GPUType != "" {
		for _, gpuType := range c.config.GPU.AvailableTypes {
			if gpuType.Name != spec.GPUType {
				continue
			}
			if nodeSelector == nil {
				nodeSelector = make(map[string]string)
			}
			for k, v := range gpuType.NodeSelector {
				nodeSelector[k] = v
			}
			break
		}
	}

	return &WorkloadRuntime{
		Container:        container,
		Volumes:          volumes,
		RuntimeClassName: runtimeClassName,
		NodeSelector:     nodeSelector,
		Affinity:         buildPodAffinity(c.config.Pod.Affinity, spec.NodeName),
		HostNetwork:      c.config.Pod.HostNetwork,
		DNSPolicy:        c.config.Pod.DNSPolicy,
		DNSConfig:        c.config.Pod.DNSConfig,
	}, nil
}

func wrapStartupScriptWithNodeRank(script string) string {
	return fmt.Sprintf(`
if [ -n "${POD_NAME:-}" ]; then
  export NODE_RANK="${POD_NAME##*-}"
fi
%s
`, script)
}

func wrapStartupScriptWithSharedDeviceAssignment(script, acceleratorType string, devicesPerReplica, totalDevices int) string {
	var exportLines string
	switch acceleratorType {
	case "ascend":
		exportLines = `
  export ASCEND_RT_VISIBLE_DEVICES="$GENET_VISIBLE_DEVICES"
  export ASCEND_VISIBLE_DEVICES="$GENET_VISIBLE_DEVICES"
`
	default:
		exportLines = `
  export NVIDIA_VISIBLE_DEVICES="$GENET_VISIBLE_DEVICES"
`
	}

	return fmt.Sprintf(`
if [ %d -gt 0 ] && [ %d -gt 0 ]; then
  rank="${NODE_RANK:-}"
  if [ -z "$rank" ] && [ -n "${POD_NAME:-}" ]; then
    rank="$(printf '%%s' "$POD_NAME" | cksum | awk '{print $1}')"
  fi
  if [ -z "$rank" ]; then
    rank="0"
  fi
  devices=""
  i=0
  while [ "$i" -lt %d ]; do
    idx=$(( (rank * %d + i) %% %d ))
    if [ -n "$devices" ]; then
      devices="${devices},${idx}"
    else
      devices="${idx}"
    fi
    i=$((i + 1))
  done
  export GENET_VISIBLE_DEVICES="$devices"
%s
fi
%s
`, devicesPerReplica, totalDevices, devicesPerReplica, devicesPerReplica, totalDevices, exportLines, script)
}
