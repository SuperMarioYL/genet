package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CommitSpec 镜像 commit 规格
type CommitSpec struct {
	PodName     string // 源 Pod 名称
	Namespace   string // Pod 所在 namespace
	Username    string // 用户名
	TargetImage string // 目标镜像名称（包含 tag）
	NodeName    string // Pod 所在节点
	ContainerID string // 容器 ID（用于 nerdctl commit）
}

// CommitJobStatus commit job 状态
type CommitJobStatus struct {
	JobName     string `json:"jobName"`
	Status      string `json:"status"`      // Pending, Running, Succeeded, Failed
	Message     string `json:"message"`     // 状态消息
	StartTime   string `json:"startTime"`   // 开始时间
	EndTime     string `json:"endTime"`     // 结束时间
	TargetImage string `json:"targetImage"` // 目标镜像名称
}

// CreateCommitJob 创建镜像 commit Job
func (c *Client) CreateCommitJob(ctx context.Context, spec *CommitSpec) (*batchv1.Job, error) {
	jobName := fmt.Sprintf("commit-%s-%d", spec.Username, time.Now().Unix())
	namespace := spec.Namespace

	// 构建 docker config.json 用于 registry 认证
	dockerConfigJSON := c.buildDockerConfigJSON()

	// 创建 registry 认证 Secret（如果配置了）
	secretName := ""
	if c.config.Registry.URL != "" && c.config.Registry.Username != "" {
		secretName = fmt.Sprintf("registry-auth-%s", spec.Username)
		if err := c.ensureRegistrySecret(ctx, namespace, secretName, dockerConfigJSON); err != nil {
			return nil, fmt.Errorf("创建 registry secret 失败: %w", err)
		}
	}

	// 构建 commit 脚本
	commitScript := c.buildCommitScript(spec, secretName != "")

	// TTL 设置：Job 完成后 10 分钟自动清理
	ttlSeconds := int32(600)
	backoffLimit := int32(0) // 不重试

	// 构建 Job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"genet.io/type":    "commit",
				"genet.io/user":    spec.Username,
				"genet.io/pod":     spec.PodName,
				"genet.io/managed": "true",
			},
			Annotations: map[string]string{
				"genet.io/target-image": spec.TargetImage,
				"genet.io/source-pod":   spec.PodName,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttlSeconds,
			BackoffLimit:            &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					NodeName:      spec.NodeName, // 必须在目标 Pod 所在节点运行
					Containers: []corev1.Container{
						{
							Name:    "commit",
							Image:   c.config.Images.Nerdctl,
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{commitScript},
							SecurityContext: &corev1.SecurityContext{
								Privileged: boolPtr(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "containerd-sock",
									MountPath: "/run/containerd/containerd.sock",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "containerd-sock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/run/containerd/containerd.sock",
								},
							},
						},
					},
				},
			},
		},
	}

	// 如果有 registry 认证，添加 secret 挂载
	if secretName != "" {
		job.Spec.Template.Spec.Containers[0].VolumeMounts = append(
			job.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "docker-config",
				MountPath: "/root/.docker",
			},
		)
		job.Spec.Template.Spec.Volumes = append(
			job.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "docker-config",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secretName,
						Items: []corev1.KeyToPath{
							{
								Key:  ".dockerconfigjson",
								Path: "config.json",
							},
						},
					},
				},
			},
		)
	}

	return c.clientset.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
}

// GetCommitJobStatus 获取 commit job 状态
func (c *Client) GetCommitJobStatus(ctx context.Context, namespace, podName string) (*CommitJobStatus, error) {
	// 查找与 pod 相关的最新 commit job
	jobs, err := c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("genet.io/type=commit,genet.io/pod=%s", podName),
	})
	if err != nil {
		return nil, fmt.Errorf("获取 commit job 列表失败: %w", err)
	}

	if len(jobs.Items) == 0 {
		return nil, nil // 没有 commit job
	}

	// 获取最新的 job
	var latestJob *batchv1.Job
	for i := range jobs.Items {
		if latestJob == nil || jobs.Items[i].CreationTimestamp.After(latestJob.CreationTimestamp.Time) {
			latestJob = &jobs.Items[i]
		}
	}

	status := &CommitJobStatus{
		JobName:     latestJob.Name,
		TargetImage: latestJob.Annotations["genet.io/target-image"],
	}

	// 解析状态
	if latestJob.Status.Succeeded > 0 {
		status.Status = "Succeeded"
		status.Message = "镜像构建并推送成功"
	} else if latestJob.Status.Failed > 0 {
		status.Status = "Failed"
		status.Message = "镜像构建失败"
	} else if latestJob.Status.Active > 0 {
		status.Status = "Running"
		status.Message = "正在构建镜像..."
	} else {
		status.Status = "Pending"
		status.Message = "等待调度..."
	}

	if latestJob.Status.StartTime != nil {
		status.StartTime = latestJob.Status.StartTime.Format(time.RFC3339)
	}
	if latestJob.Status.CompletionTime != nil {
		status.EndTime = latestJob.Status.CompletionTime.Format(time.RFC3339)
	}

	return status, nil
}

// GetCommitJobLogs 获取 commit job 日志
func (c *Client) GetCommitJobLogs(ctx context.Context, namespace, podName string) (string, error) {
	// 查找与 pod 相关的最新 commit job
	jobs, err := c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("genet.io/type=commit,genet.io/pod=%s", podName),
	})
	if err != nil {
		return "", fmt.Errorf("获取 commit job 列表失败: %w", err)
	}

	if len(jobs.Items) == 0 {
		return "", fmt.Errorf("没有找到相关的 commit job")
	}

	// 获取最新的 job
	var latestJob *batchv1.Job
	for i := range jobs.Items {
		if latestJob == nil || jobs.Items[i].CreationTimestamp.After(latestJob.CreationTimestamp.Time) {
			latestJob = &jobs.Items[i]
		}
	}

	// 获取 job 的 pod
	pods, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", latestJob.Name),
	})
	if err != nil {
		return "", fmt.Errorf("获取 job pod 失败: %w", err)
	}

	if len(pods.Items) == 0 {
		return "Job Pod 尚未创建", nil
	}

	// 获取 pod 日志
	jobPod := pods.Items[0]
	tailLines := int64(200)
	req := c.clientset.CoreV1().Pods(namespace).GetLogs(jobPod.Name, &corev1.PodLogOptions{
		TailLines: &tailLines,
	})

	logs, err := req.Do(ctx).Raw()
	if err != nil {
		return "", fmt.Errorf("获取日志失败: %w", err)
	}

	return string(logs), nil
}

// buildCommitScript 构建 commit 脚本
func (c *Client) buildCommitScript(spec *CommitSpec, hasAuth bool) string {
	// 如果配置了 insecure，添加 --insecure-registry 参数
	// 注意：这个参数需要宿主机 containerd 也配置了 insecure registry 才能生效
	// 参考文档：需要在 K8s 节点上配置 /etc/containerd/certs.d/<registry>/hosts.toml
	insecureFlag := ""
	if c.config.Registry.Insecure {
		insecureFlag = "--insecure-registry"
	}

	script := fmt.Sprintf(`
set -e
echo "=== Genet Image Commit ==="
echo "Source Pod: %s"
echo "Target Image: %s"
echo "Node: %s"
echo "Insecure Registry: %s"

# 查找容器 ID
echo "Searching for container..."

# containerd k8s 命名空间中的容器名格式: k8s://<namespace>/<pod>/<container> 或 <pod>_<namespace>_<container>
# 先列出所有容器看看格式
echo "Listing containers in k8s.io namespace..."
nerdctl -n k8s.io ps -a 2>/dev/null || true

# 方式1: 通过 Pod 名称匹配（包含 pod 名称的容器，排除 POD sandbox）
CONTAINER_ID=$(nerdctl -n k8s.io ps -a --format '{{.ID}} {{.Names}}' 2>/dev/null | grep -i '%s' | grep -vi 'POD' | grep -vi 'pause' | head -1 | awk '{print $1}' || true)

# 方式2: 如果方式1没找到，尝试用更宽松的匹配
if [ -z "$CONTAINER_ID" ]; then
    echo "Trying alternative search..."
    # 查找 workspace 容器
    CONTAINER_ID=$(nerdctl -n k8s.io ps -a --format '{{.ID}} {{.Names}}' 2>/dev/null | grep -i 'workspace' | head -1 | awk '{print $1}' || true)
fi

# 方式3: 如果还是没找到，列出所有容器供调试
if [ -z "$CONTAINER_ID" ]; then
    echo ""
    echo "ERROR: Container not found for pod %s"
    echo ""
    echo "Debug info - all containers:"
    echo "================================"
    nerdctl -n k8s.io ps -a 2>/dev/null || echo "Failed to list containers"
    echo ""
    echo "Try checking Docker namespace:"
    nerdctl ps -a 2>/dev/null || true
    exit 1
fi

echo ""
echo "Found container ID: $CONTAINER_ID"
echo ""

# Commit 容器为镜像
echo "Committing container to image: %s"
nerdctl -n k8s.io commit "$CONTAINER_ID" %s

echo ""
echo "Commit successful!"
echo ""

# 推送镜像
echo "Pushing image to registry: %s"
nerdctl -n k8s.io %s push %s

echo ""
echo "=== SUCCESS ==="
echo "Image %s has been pushed successfully!"
`, spec.PodName, spec.TargetImage, spec.NodeName,
		insecureFlag,                       // 显示是否使用 insecure
		spec.PodName,                       // 方式1 grep
		spec.PodName,                       // 错误信息
		spec.TargetImage, spec.TargetImage, // commit: echo, nerdctl commit
		spec.TargetImage, insecureFlag, spec.TargetImage, // push: echo, insecureFlag, 镜像名
		spec.TargetImage) // 成功信息

	return script
}

// buildDockerConfigJSON 构建 docker config.json
func (c *Client) buildDockerConfigJSON() string {
	if c.config.Registry.URL == "" || c.config.Registry.Username == "" {
		return ""
	}

	auth := base64.StdEncoding.EncodeToString(
		[]byte(fmt.Sprintf("%s:%s", c.config.Registry.Username, c.config.Registry.Password)),
	)

	config := map[string]interface{}{
		"auths": map[string]interface{}{
			c.config.Registry.URL: map[string]string{
				"auth": auth,
			},
		},
	}

	data, _ := json.Marshal(config)
	return string(data)
}

// ensureRegistrySecret 确保 registry 认证 Secret 存在
func (c *Client) ensureRegistrySecret(ctx context.Context, namespace, secretName, dockerConfigJSON string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"genet.io/type":    "registry-auth",
				"genet.io/managed": "true",
			},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			".dockerconfigjson": []byte(dockerConfigJSON),
		},
	}

	_, err := c.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// 更新已存在的 secret
			_, err = c.clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
		}
	}
	return err
}

// boolPtr 返回 bool 指针
func boolPtr(b bool) *bool {
	return &b
}
