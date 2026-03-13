package k8s

import (
	"context"
	"fmt"

	"github.com/uc-package/genet/internal/models"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) BuildJobFromOpenAPIRequest(ctx context.Context, namespace, ownerUser string, req *models.OpenAPIJobRequest) (*batchv1.Job, error) {
	if req == nil {
		return nil, fmt.Errorf("job 请求不能为空")
	}

	runtimeSpec, err := c.buildWorkloadRuntime(ctx, &WorkloadRuntimeSpec{
		Name:       req.Name,
		Username:   ownerUser,
		Image:      req.Image,
		CPU:        req.CPU,
		Memory:     req.Memory,
		ShmSize:    req.ShmSize,
		Command:    req.Command,
		Args:       req.Args,
		WorkingDir: req.WorkingDir,
		Env:        buildOpenAPIEnvVars(req.Env),
		NodeName:   req.NodeName,
		GPUCount:   req.GPUCount,
		GPUType:    req.GPUType,
		GPUDevices: req.GPUDevices,
		UserMounts: req.UserMounts,
	})
	if err != nil {
		return nil, err
	}

	restartPolicy := corev1.RestartPolicyNever
	if req.RestartPolicy != "" {
		restartPolicy = corev1.RestartPolicy(req.RestartPolicy)
	}

	jobLabels := map[string]string{
		"genet.io/open-api":      "true",
		"genet.io/managed":       "true",
		"genet.io/openapi-owner": ownerUser,
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   namespace,
			Labels:      jobLabels,
			Annotations: req.Annotations,
		},
		Spec: batchv1.JobSpec{
			Parallelism:             req.Parallelism,
			Completions:             req.Completions,
			BackoffLimit:            req.BackoffLimit,
			TTLSecondsAfterFinished: req.TTLSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: jobLabels,
				},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: boolPtr(false),
					HostNetwork:                  runtimeSpec.HostNetwork,
					RestartPolicy:                restartPolicy,
					Containers:                   []corev1.Container{runtimeSpec.Container},
					Volumes:                      runtimeSpec.Volumes,
					NodeSelector:                 runtimeSpec.NodeSelector,
					Affinity:                     runtimeSpec.Affinity,
					DNSPolicy:                    runtimeSpec.DNSPolicy,
					DNSConfig:                    runtimeSpec.DNSConfig,
				},
			},
		},
	}

	if runtimeSpec.RuntimeClassName != nil {
		job.Spec.Template.Spec.RuntimeClassName = runtimeSpec.RuntimeClassName
	}

	return job, nil
}

func buildOpenAPIEnvVars(envs []models.OpenAPIEnvVar) []corev1.EnvVar {
	if len(envs) == 0 {
		return nil
	}

	result := make([]corev1.EnvVar, 0, len(envs))
	for _, env := range envs {
		result = append(result, corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
	}
	return result
}

func (c *Client) CreateJob(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error) {
	return c.clientset.BatchV1().Jobs(job.Namespace).Create(ctx, job, metav1.CreateOptions{})
}

func (c *Client) ListJobs(ctx context.Context, namespace, labelSelector string) (*batchv1.JobList, error) {
	return c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

func (c *Client) GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error) {
	return c.clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) DeleteJob(ctx context.Context, namespace, name string) error {
	propagation := metav1.DeletePropagationBackground
	return c.clientset.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
}
