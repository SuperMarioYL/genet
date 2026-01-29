package handlers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
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
