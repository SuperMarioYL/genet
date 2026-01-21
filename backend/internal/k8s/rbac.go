package k8s

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UserRBACConfig 用户 RBAC 配置
type UserRBACConfig struct {
	// 用户名（OIDC username claim）
	Username string
	// 用户邮箱（可选，用于 OIDC 认证时的用户标识）
	Email string
	// 用户 Namespace
	Namespace string
}

// EnsureUserRBAC 确保用户的 RBAC 资源存在
// 创建 Role 和 RoleBinding，让用户可以通过 kubectl 访问自己的资源
func (c *Client) EnsureUserRBAC(ctx context.Context, cfg UserRBACConfig) error {
	c.log.Info("Ensuring user RBAC",
		zap.String("username", cfg.Username),
		zap.String("namespace", cfg.Namespace),
		zap.String("email", cfg.Email))

	// 1. 确保 Namespace 存在
	if err := c.EnsureNamespace(ctx, cfg.Namespace); err != nil {
		c.log.Error("Failed to ensure namespace for RBAC",
			zap.String("namespace", cfg.Namespace),
			zap.Error(err))
		return fmt.Errorf("创建 Namespace 失败: %w", err)
	}

	// 2. 创建 Role
	if err := c.ensureUserRole(ctx, cfg); err != nil {
		c.log.Error("Failed to ensure user role",
			zap.String("username", cfg.Username),
			zap.Error(err))
		return fmt.Errorf("创建 Role 失败: %w", err)
	}

	// 3. 创建 RoleBinding
	if err := c.ensureUserRoleBinding(ctx, cfg); err != nil {
		c.log.Error("Failed to ensure user role binding",
			zap.String("username", cfg.Username),
			zap.Error(err))
		return fmt.Errorf("创建 RoleBinding 失败: %w", err)
	}

	c.log.Info("User RBAC ensured successfully",
		zap.String("username", cfg.Username),
		zap.String("namespace", cfg.Namespace))

	return nil
}

// ensureUserRole 创建用户在其 Namespace 内的 Role
func (c *Client) ensureUserRole(ctx context.Context, cfg UserRBACConfig) error {
	roleName := fmt.Sprintf("user-%s-role", cfg.Username)

	c.log.Debug("Ensuring user role",
		zap.String("roleName", roleName),
		zap.String("namespace", cfg.Namespace))

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				"genet.io/managed":  "true",
				"genet.io/username": cfg.Username,
			},
		},
		Rules: []rbacv1.PolicyRule{
			// Pod 完全权限
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch", "create", "delete", "patch", "update"},
			},
			// Pod 日志和 exec
			{
				APIGroups: []string{""},
				Resources: []string{"pods/log", "pods/exec", "pods/attach", "pods/portforward"},
				Verbs:     []string{"get", "create"},
			},
			// PVC 权限
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims"},
				Verbs:     []string{"get", "list", "watch", "create", "delete"},
			},
			// ConfigMap 和 Secret 只读
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps", "secrets"},
				Verbs:     []string{"get", "list", "watch"},
			},
			// Service 权限（用于暴露服务）
			{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"get", "list", "watch", "create", "delete"},
			},
			// Events 只读（用于调试）
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}

	_, err := c.clientset.RbacV1().Roles(cfg.Namespace).Create(ctx, role, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			c.log.Debug("Role already exists, updating",
				zap.String("roleName", roleName))
			// 更新已存在的 Role
			_, err = c.clientset.RbacV1().Roles(cfg.Namespace).Update(ctx, role, metav1.UpdateOptions{})
		}
		if err != nil {
			return err
		}
	} else {
		c.log.Debug("Role created",
			zap.String("roleName", roleName),
			zap.String("namespace", cfg.Namespace))
	}

	return nil
}

// ensureUserRoleBinding 创建 RoleBinding，绑定 OIDC 用户到 Role
func (c *Client) ensureUserRoleBinding(ctx context.Context, cfg UserRBACConfig) error {
	roleName := fmt.Sprintf("user-%s-role", cfg.Username)
	bindingName := fmt.Sprintf("user-%s-binding", cfg.Username)

	c.log.Debug("Ensuring user role binding",
		zap.String("bindingName", bindingName),
		zap.String("roleName", roleName),
		zap.String("namespace", cfg.Namespace))

	// 确定用户标识（OIDC 中的 username claim）
	// K8s 中的 User 名称需要与 OIDC token 中的 username claim 一致
	userName := cfg.Username

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				"genet.io/managed":  "true",
				"genet.io/username": cfg.Username,
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     userName, // 对应 OIDC 的 username claim
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     roleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	_, err := c.clientset.RbacV1().RoleBindings(cfg.Namespace).Create(ctx, binding, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			c.log.Debug("RoleBinding already exists, updating",
				zap.String("bindingName", bindingName))
			// 更新已存在的 RoleBinding
			_, err = c.clientset.RbacV1().RoleBindings(cfg.Namespace).Update(ctx, binding, metav1.UpdateOptions{})
		}
		if err != nil {
			return err
		}
	} else {
		c.log.Debug("RoleBinding created",
			zap.String("bindingName", bindingName),
			zap.String("namespace", cfg.Namespace),
			zap.String("user", userName))
	}

	return nil
}

// DeleteUserRBAC 删除用户的 RBAC 资源（可选，用于清理）
func (c *Client) DeleteUserRBAC(ctx context.Context, username, namespace string) error {
	roleName := fmt.Sprintf("user-%s-role", username)
	bindingName := fmt.Sprintf("user-%s-binding", username)

	c.log.Info("Deleting user RBAC",
		zap.String("username", username),
		zap.String("namespace", namespace))

	// 删除 RoleBinding
	err := c.clientset.RbacV1().RoleBindings(namespace).Delete(ctx, bindingName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		c.log.Error("Failed to delete RoleBinding",
			zap.String("bindingName", bindingName),
			zap.Error(err))
		return fmt.Errorf("删除 RoleBinding 失败: %w", err)
	}

	// 删除 Role
	err = c.clientset.RbacV1().Roles(namespace).Delete(ctx, roleName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		c.log.Error("Failed to delete Role",
			zap.String("roleName", roleName),
			zap.Error(err))
		return fmt.Errorf("删除 Role 失败: %w", err)
	}

	c.log.Info("User RBAC deleted successfully",
		zap.String("username", username),
		zap.String("namespace", namespace))

	return nil
}

// ListUserRBAC 列出用户的 RBAC 资源
func (c *Client) ListUserRBAC(ctx context.Context, namespace string) ([]rbacv1.Role, []rbacv1.RoleBinding, error) {
	c.log.Debug("Listing user RBAC",
		zap.String("namespace", namespace))

	// 列出 Genet 管理的 Role
	roles, err := c.clientset.RbacV1().Roles(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "genet.io/managed=true",
	})
	if err != nil {
		c.log.Error("Failed to list Roles",
			zap.String("namespace", namespace),
			zap.Error(err))
		return nil, nil, fmt.Errorf("列出 Role 失败: %w", err)
	}

	// 列出 Genet 管理的 RoleBinding
	bindings, err := c.clientset.RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "genet.io/managed=true",
	})
	if err != nil {
		c.log.Error("Failed to list RoleBindings",
			zap.String("namespace", namespace),
			zap.Error(err))
		return nil, nil, fmt.Errorf("列出 RoleBinding 失败: %w", err)
	}

	c.log.Debug("Listed user RBAC",
		zap.String("namespace", namespace),
		zap.Int("roles", len(roles.Items)),
		zap.Int("bindings", len(bindings.Items)))

	return roles.Items, bindings.Items, nil
}
