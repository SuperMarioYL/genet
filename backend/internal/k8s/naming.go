package k8s

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// K8s 名称合法字符：小写字母、数字、连字符
var k8sNameRegex = regexp.MustCompile(`[^a-z0-9-]`)

// GetUserIdentifier 从用户名和邮箱生成用户标识
// 格式: {username}-{emailPrefix}（username 在前），无邮箱时只用 username
func GetUserIdentifier(username, email string) string {
	var parts []string

	// username 在前
	if username != "" {
		sanitizedUsername := SanitizeK8sName(username)
		if sanitizedUsername != "" {
			parts = append(parts, sanitizedUsername)
		}
	}

	// 邮箱前缀在后
	if email != "" {
		emailParts := strings.Split(email, "@")
		if len(emailParts) > 0 && emailParts[0] != "" {
			emailPrefix := SanitizeK8sName(emailParts[0])
			if emailPrefix != "" {
				parts = append(parts, emailPrefix)
			}
		}
	}

	identifier := strings.Join(parts, "-")

	// 限制总长度（K8s 名称最长 63 字符，预留前缀空间）
	if len(identifier) > 40 {
		identifier = identifier[:40]
		identifier = strings.TrimRight(identifier, "-")
	}

	return identifier
}

// SanitizeK8sName 清理名称使其符合 K8s 命名规范
// - 转小写
// - 点号转连字符
// - 下划线转连字符
// - 移除非法字符
// - 确保以字母或数字开头和结尾
func SanitizeK8sName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")
	name = k8sNameRegex.ReplaceAllString(name, "")

	// 移除开头和结尾的连字符
	name = strings.Trim(name, "-")

	// 合并连续的连字符
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	// 限制长度
	if len(name) > 40 {
		name = name[:40]
		name = strings.TrimRight(name, "-")
	}

	return name
}

// GeneratePodName 生成 Pod 名称
// userIdentifier: 用户标识（username-emailPrefix）
// customName: 用户自定义名称（可选），为空则使用时间戳
func GeneratePodName(userIdentifier, customName string) string {
	if customName != "" {
		customName = SanitizeK8sName(customName)
		return fmt.Sprintf("pod-%s-%s", userIdentifier, customName)
	}
	return fmt.Sprintf("pod-%s-%d", userIdentifier, time.Now().Unix())
}

// GenerateJobName 生成 Job 名称
// prefix: 前缀，如 "commit"
// userIdentifier: 用户标识
func GenerateJobName(prefix, userIdentifier string) string {
	return fmt.Sprintf("%s-%s-%d", prefix, userIdentifier, time.Now().Unix())
}

// GetNamespaceForUserIdentifier 根据用户标识获取 Namespace 名称
func GetNamespaceForUserIdentifier(userIdentifier string) string {
	return fmt.Sprintf("user-%s", userIdentifier)
}

// ValidatePodCustomName 验证自定义 Pod 名称
func ValidatePodCustomName(name string) error {
	if name == "" {
		return nil
	}

	// 长度限制
	if len(name) > 20 {
		return fmt.Errorf("Pod 名称最多 20 个字符")
	}

	// 格式验证：只允许小写字母、数字、连字符
	validPattern := regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)
	if !validPattern.MatchString(name) {
		return fmt.Errorf("Pod 名称只能包含小写字母、数字和连字符，且不能以连字符开头或结尾")
	}

	return nil
}
