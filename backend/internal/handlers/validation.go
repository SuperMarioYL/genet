package handlers

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/uc-package/genet/internal/models"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
)

// 镜像名称正则：允许 registry/repo:tag 格式
var imageNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._\-/:]*[a-zA-Z0-9]$`)

// CPU 格式正则：纯数字或带 m 后缀
var cpuRegex = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(m)?$`)

// 内存格式正则：数字 + 单位 (Ki, Mi, Gi, Ti)
var memoryRegex = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(Ki|Mi|Gi|Ti)?$`)

// ValidateImageName 验证镜像名称
func ValidateImageName(image string) error {
	if image == "" {
		return fmt.Errorf("镜像名称不能为空")
	}
	if len(image) > 255 {
		return fmt.Errorf("镜像名称过长（最大 255 字符）")
	}
	if !imageNameRegex.MatchString(image) {
		return fmt.Errorf("镜像名称格式无效")
	}
	// 检查危险字符
	if strings.Contains(image, "..") || strings.Contains(image, "//") {
		return fmt.Errorf("镜像名称包含非法字符")
	}
	return nil
}

// ValidateCPU 验证 CPU 请求格式
func ValidateCPU(cpu string) error {
	if cpu == "" {
		return nil // 可选字段
	}
	if !cpuRegex.MatchString(cpu) {
		return fmt.Errorf("CPU 格式无效，应为数字或带 m 后缀（如 4, 500m）")
	}
	// 检查合理范围
	value := strings.TrimSuffix(cpu, "m")
	num, _ := strconv.ParseFloat(value, 64)
	if strings.HasSuffix(cpu, "m") {
		num = num / 1000
	}
	if num <= 0 || num > 128 {
		return fmt.Errorf("CPU 值超出合理范围 (0-128)")
	}
	return nil
}

// ValidateMemory 验证内存请求格式
func ValidateMemory(memory string) error {
	if memory == "" {
		return nil // 可选字段
	}
	if !memoryRegex.MatchString(memory) {
		return fmt.Errorf("内存格式无效，应为数字+单位（如 4Gi, 512Mi）")
	}
	return nil
}

func ValidateOpenAPIServiceRequest(req *models.OpenAPIServiceRequest) error {
	if req == nil {
		return fmt.Errorf("service 请求不能为空")
	}
	if err := validateK8sResourceName(req.Name, "Service"); err != nil {
		return err
	}
	if err := validateReservedAnnotations(req.Annotations); err != nil {
		return err
	}

	serviceType := req.Type
	if serviceType == "" {
		serviceType = "ClusterIP"
	}
	switch serviceType {
	case "ClusterIP", "NodePort", "LoadBalancer":
	default:
		return fmt.Errorf("不支持的 Service 类型: %s", serviceType)
	}

	if req.TargetPodName == "" && len(req.Selector) == 0 {
		return fmt.Errorf("selector 和 targetPodName 不能同时为空")
	}
	if req.TargetPodName != "" && len(req.Selector) > 0 {
		return fmt.Errorf("selector 和 targetPodName 只能指定一个")
	}
	if len(req.Ports) == 0 {
		return fmt.Errorf("Service 至少需要一个端口")
	}

	for _, port := range req.Ports {
		if port.Port <= 0 || port.Port > 65535 {
			return fmt.Errorf("Service 端口超出范围: %d", port.Port)
		}
		if port.Protocol != "" {
			switch port.Protocol {
			case "TCP", "UDP", "SCTP":
			default:
				return fmt.Errorf("不支持的 Service 协议: %s", port.Protocol)
			}
		}
		if port.NodePort != 0 && serviceType != "NodePort" && serviceType != "LoadBalancer" {
			return fmt.Errorf("nodePort 仅允许用于 NodePort 或 LoadBalancer Service")
		}
	}

	return nil
}

func ValidateOpenAPIConfigMapRequest(req *models.OpenAPIConfigMapRequest) error {
	if req == nil {
		return fmt.Errorf("configmap 请求不能为空")
	}
	if err := validateK8sResourceName(req.Name, "ConfigMap"); err != nil {
		return err
	}
	if err := validateReservedAnnotations(req.Annotations); err != nil {
		return err
	}
	if len(req.Data) == 0 && len(req.BinaryData) == 0 {
		return fmt.Errorf("data 和 binaryData 不能同时为空")
	}
	for key, value := range req.BinaryData {
		if key == "" {
			return fmt.Errorf("binaryData 键不能为空")
		}
		if _, err := base64.StdEncoding.DecodeString(value); err != nil {
			return fmt.Errorf("binaryData %s 不是合法 base64", key)
		}
	}
	return nil
}

func ValidateOpenAPIJobRequest(req *models.OpenAPIJobRequest) error {
	if req == nil {
		return fmt.Errorf("job 请求不能为空")
	}
	if err := validateK8sResourceName(req.Name, "Job"); err != nil {
		return err
	}
	if err := ValidateImageName(req.Image); err != nil {
		return err
	}
	if err := ValidateCPU(req.CPU); err != nil {
		return err
	}
	if err := ValidateMemory(req.Memory); err != nil {
		return err
	}
	if err := ValidateMemory(req.ShmSize); err != nil {
		return fmt.Errorf("共享内存格式无效，应为数字+单位（如 1Gi, 512Mi）")
	}
	if err := validateReservedAnnotations(req.Annotations); err != nil {
		return err
	}

	if req.RestartPolicy != "" && req.RestartPolicy != "Never" && req.RestartPolicy != "OnFailure" {
		return fmt.Errorf("restartPolicy 仅支持 Never 或 OnFailure")
	}
	if len(req.GPUDevices) > 0 && req.NodeName == "" {
		return fmt.Errorf("指定 GPU 卡时必须同时指定节点")
	}
	if req.Parallelism != nil && *req.Parallelism < 0 {
		return fmt.Errorf("parallelism 不能为负数")
	}
	if req.Completions != nil && *req.Completions < 0 {
		return fmt.Errorf("completions 不能为负数")
	}
	if req.BackoffLimit != nil && *req.BackoffLimit < 0 {
		return fmt.Errorf("backoffLimit 不能为负数")
	}
	if req.TTLSecondsAfterFinished != nil && *req.TTLSecondsAfterFinished < 0 {
		return fmt.Errorf("ttlSecondsAfterFinished 不能为负数")
	}
	for _, env := range req.Env {
		if strings.TrimSpace(env.Name) == "" {
			return fmt.Errorf("环境变量名称不能为空")
		}
	}
	return nil
}

func validateK8sResourceName(name, resourceType string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%s 名称不能为空", resourceType)
	}
	if errs := k8svalidation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("%s 名称不合法: %s", resourceType, strings.Join(errs, ", "))
	}
	return nil
}

func validateReservedAnnotations(annotations map[string]string) error {
	for key := range annotations {
		if strings.HasPrefix(key, "genet.io/") {
			return fmt.Errorf("注解 %s 为系统保留前缀", key)
		}
	}
	return nil
}
