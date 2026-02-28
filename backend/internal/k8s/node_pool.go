package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const (
	defaultNonSharedLabelKey   = "genet.io/node-pool"
	defaultNonSharedLabelValue = "non-shared"
	defaultNonSharedTaintKey   = "genet.io/non-shared-pool"
	defaultNonSharedTaintValue = "true"
	defaultNodePoolSyncSeconds = 60
)

type resolvedNodePoolConfig struct {
	enabled bool

	nonSharedLabelKey   string
	nonSharedLabelValue string

	nonSharedTaintKey    string
	nonSharedTaintValue  string
	nonSharedTaintEffect corev1.TaintEffect

	syncInterval time.Duration
}

// StartNodePoolTaintReconciler 启动节点池污点同步协程
func (c *Client) StartNodePoolTaintReconciler(ctx context.Context) {
	cfg := resolveNodePoolConfig(c.config.GPU.NodePool)
	if !cfg.enabled {
		c.log.Info("Node pool taint reconciler disabled")
		return
	}

	c.log.Info("Starting node pool taint reconciler",
		zap.String("labelKey", cfg.nonSharedLabelKey),
		zap.String("labelValue", cfg.nonSharedLabelValue),
		zap.String("taintKey", cfg.nonSharedTaintKey),
		zap.String("taintValue", cfg.nonSharedTaintValue),
		zap.String("taintEffect", string(cfg.nonSharedTaintEffect)),
		zap.Duration("syncInterval", cfg.syncInterval))

	go func() {
		if err := c.syncNodePoolTaints(ctx, cfg); err != nil {
			c.log.Warn("Initial node pool taint sync failed", zap.Error(err))
		}

		ticker := time.NewTicker(cfg.syncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				c.log.Info("Node pool taint reconciler stopped")
				return
			case <-ticker.C:
				if err := c.syncNodePoolTaints(ctx, cfg); err != nil {
					c.log.Warn("Node pool taint sync failed", zap.Error(err))
				}
			}
		}
	}()
}

// SyncNodePoolTaints 同步节点池污点（一次性执行）
func (c *Client) SyncNodePoolTaints(ctx context.Context) error {
	cfg := resolveNodePoolConfig(c.config.GPU.NodePool)
	if !cfg.enabled {
		return nil
	}
	return c.syncNodePoolTaints(ctx, cfg)
}

func (c *Client) syncNodePoolTaints(ctx context.Context, cfg resolvedNodePoolConfig) error {
	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes failed: %w", err)
	}

	var errMsgs []string
	updatedCount := 0
	for _, node := range nodes.Items {
		changed, action, err := c.reconcileNodePoolTaint(ctx, node.Name, cfg)
		if err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", node.Name, err))
			continue
		}
		if changed {
			updatedCount++
			c.log.Info("Node pool taint reconciled",
				zap.String("node", node.Name),
				zap.String("action", action))
		}
	}

	if updatedCount > 0 {
		c.log.Info("Node pool taint sync completed", zap.Int("updatedNodes", updatedCount))
	}
	if len(errMsgs) > 0 {
		return fmt.Errorf("node pool taint sync partial failure (%d): %s", len(errMsgs), strings.Join(errMsgs, "; "))
	}
	return nil
}

func (c *Client) reconcileNodePoolTaint(ctx context.Context, nodeName string, cfg resolvedNodePoolConfig) (bool, string, error) {
	changed := false
	action := ""

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		node = node.DeepCopy()
		changedNow, actionNow := applyNodePoolTaint(node, cfg)
		if !changedNow {
			return nil
		}

		if _, err := c.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{}); err != nil {
			return err
		}

		changed = true
		action = actionNow
		return nil
	})
	if err != nil {
		return false, "", err
	}
	return changed, action, nil
}

func resolveNodePoolConfig(cfg models.NodePoolConfig) resolvedNodePoolConfig {
	enabled := cfg.Enabled
	if !enabled && (cfg.NonSharedLabelKey != "" || cfg.NonSharedLabelValue != "" || cfg.NonSharedTaintKey != "" || cfg.NonSharedTaintValue != "" || cfg.NonSharedTaintEffect != "" || cfg.SyncIntervalSeconds > 0) {
		enabled = true
	}

	labelKey := cfg.NonSharedLabelKey
	if labelKey == "" {
		labelKey = defaultNonSharedLabelKey
	}

	labelValue := cfg.NonSharedLabelValue
	if labelValue == "" {
		labelValue = defaultNonSharedLabelValue
	}

	taintKey := cfg.NonSharedTaintKey
	if taintKey == "" {
		taintKey = defaultNonSharedTaintKey
	}

	taintValue := cfg.NonSharedTaintValue
	if taintValue == "" {
		taintValue = defaultNonSharedTaintValue
	}

	syncSeconds := cfg.SyncIntervalSeconds
	if syncSeconds <= 0 {
		syncSeconds = defaultNodePoolSyncSeconds
	}

	return resolvedNodePoolConfig{
		enabled:              enabled,
		nonSharedLabelKey:    labelKey,
		nonSharedLabelValue:  labelValue,
		nonSharedTaintKey:    taintKey,
		nonSharedTaintValue:  taintValue,
		nonSharedTaintEffect: parseTaintEffect(cfg.NonSharedTaintEffect),
		syncInterval:         time.Duration(syncSeconds) * time.Second,
	}
}

func parseTaintEffect(effect string) corev1.TaintEffect {
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case strings.ToLower(string(corev1.TaintEffectNoExecute)):
		return corev1.TaintEffectNoExecute
	case strings.ToLower(string(corev1.TaintEffectPreferNoSchedule)):
		return corev1.TaintEffectPreferNoSchedule
	default:
		return corev1.TaintEffectNoSchedule
	}
}

func applyNodePoolTaint(node *corev1.Node, cfg resolvedNodePoolConfig) (bool, string) {
	isNonShared := isNonSharedPoolNode(node, cfg)
	if isNonShared {
		return ensureNodeHasNonSharedTaint(node, cfg)
	}
	return ensureNodeHasNoNonSharedTaint(node, cfg)
}

func isNonSharedPoolNode(node *corev1.Node, cfg resolvedNodePoolConfig) bool {
	if cfg.nonSharedLabelKey == "" || len(node.Labels) == 0 {
		return false
	}
	value, ok := node.Labels[cfg.nonSharedLabelKey]
	if !ok {
		return false
	}
	if cfg.nonSharedLabelValue == "" {
		return true
	}
	return strings.TrimSpace(value) == strings.TrimSpace(cfg.nonSharedLabelValue)
}

func ensureNodeHasNonSharedTaint(node *corev1.Node, cfg resolvedNodePoolConfig) (bool, string) {
	desired := corev1.Taint{
		Key:    cfg.nonSharedTaintKey,
		Value:  cfg.nonSharedTaintValue,
		Effect: cfg.nonSharedTaintEffect,
	}

	existing := node.Spec.Taints
	filtered := make([]corev1.Taint, 0, len(existing))
	hadTaint := false
	for _, taint := range existing {
		if taint.Key == cfg.nonSharedTaintKey {
			hadTaint = true
			continue
		}
		filtered = append(filtered, taint)
	}

	filtered = append(filtered, desired)
	if hadTaint && hasEquivalentTaint(existing, desired) && countTaintsByKey(existing, cfg.nonSharedTaintKey) == 1 {
		return false, ""
	}

	node.Spec.Taints = filtered
	if hadTaint {
		return true, "updated"
	}
	return true, "added"
}

func ensureNodeHasNoNonSharedTaint(node *corev1.Node, cfg resolvedNodePoolConfig) (bool, string) {
	existing := node.Spec.Taints
	filtered := make([]corev1.Taint, 0, len(existing))
	removed := false
	for _, taint := range existing {
		if taint.Key == cfg.nonSharedTaintKey {
			removed = true
			continue
		}
		filtered = append(filtered, taint)
	}

	if !removed {
		return false, ""
	}
	node.Spec.Taints = filtered
	return true, "removed"
}

func hasEquivalentTaint(taints []corev1.Taint, target corev1.Taint) bool {
	for _, taint := range taints {
		if taint.Key == target.Key && taint.Value == target.Value && taint.Effect == target.Effect {
			return true
		}
	}
	return false
}

func countTaintsByKey(taints []corev1.Taint, key string) int {
	count := 0
	for _, taint := range taints {
		if taint.Key == key {
			count++
		}
	}
	return count
}
