package handlers

import (
	"context"
	"fmt"

	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
)

func resolveUserPoolType(ctx context.Context, client *k8s.Client, userIdentifier string) (string, error) {
	if client == nil {
		return k8s.UserPoolTypeShared, nil
	}
	record, ok, err := client.GetUserPoolBinding(ctx, userIdentifier)
	if err != nil {
		return "", err
	}
	if !ok {
		return k8s.UserPoolTypeShared, nil
	}
	return k8s.NormalizeUserPoolType(record.PoolType), nil
}

func nodePoolMatches(requestedPoolType, actualPoolType string) bool {
	requestedPoolType = k8s.NormalizeUserPoolType(requestedPoolType)
	actualPoolType = k8s.NormalizeUserPoolType(actualPoolType)
	return requestedPoolType == actualPoolType
}

func validateRequestedNodePool(node corev1.Node, config *models.Config, requestedPoolType string) error {
	actualPoolType := getNodePoolType(node, config)
	if nodePoolMatches(requestedPoolType, actualPoolType) {
		return nil
	}
	return fmt.Errorf("节点 %s 不属于当前用户可用的%s", node.Name, poolTypeLabel(requestedPoolType))
}

func poolTypeLabel(poolType string) string {
	if k8s.NormalizeUserPoolType(poolType) == k8s.UserPoolTypeExclusive {
		return "独占池"
	}
	return "共享池"
}
