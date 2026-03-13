package k8s

import (
	"context"
	"fmt"
	"strings"

	"github.com/uc-package/genet/internal/models"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func BuildServiceFromOpenAPIRequest(namespace, ownerUser string, req *models.OpenAPIServiceRequest) (*corev1.Service, error) {
	if req == nil {
		return nil, fmt.Errorf("service 请求不能为空")
	}

	serviceType := corev1.ServiceTypeClusterIP
	switch strings.TrimSpace(req.Type) {
	case "", "ClusterIP":
		serviceType = corev1.ServiceTypeClusterIP
	case "NodePort":
		serviceType = corev1.ServiceTypeNodePort
	case "LoadBalancer":
		serviceType = corev1.ServiceTypeLoadBalancer
	default:
		return nil, fmt.Errorf("不支持的 Service 类型: %s", req.Type)
	}

	selector := map[string]string{}
	if req.TargetPodName != "" {
		selector["app"] = req.TargetPodName
	} else {
		for k, v := range req.Selector {
			selector[k] = v
		}
	}

	ports := make([]corev1.ServicePort, 0, len(req.Ports))
	for _, port := range req.Ports {
		servicePort := corev1.ServicePort{
			Name:     port.Name,
			Protocol: corev1.Protocol(port.Protocol),
			Port:     port.Port,
			NodePort: port.NodePort,
		}
		if servicePort.Protocol == "" {
			servicePort.Protocol = corev1.ProtocolTCP
		}
		if port.TargetPort != "" {
			servicePort.TargetPort = intstr.Parse(port.TargetPort)
		}
		ports = append(ports, servicePort)
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: namespace,
			Labels: map[string]string{
				"genet.io/open-api":      "true",
				"genet.io/managed":       "true",
				"genet.io/openapi-owner": ownerUser,
			},
			Annotations: req.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:                     serviceType,
			Selector:                 selector,
			Ports:                    ports,
			SessionAffinity:          corev1.ServiceAffinity(req.SessionAffinity),
			PublishNotReadyAddresses: req.PublishNotReadyAddresses,
		},
	}, nil
}

func (c *Client) CreateService(ctx context.Context, service *corev1.Service) (*corev1.Service, error) {
	return c.clientset.CoreV1().Services(service.Namespace).Create(ctx, service, metav1.CreateOptions{})
}

func (c *Client) ListServices(ctx context.Context, namespace, labelSelector string) (*corev1.ServiceList, error) {
	return c.clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

func (c *Client) GetService(ctx context.Context, namespace, name string) (*corev1.Service, error) {
	return c.clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) UpdateService(ctx context.Context, service *corev1.Service) (*corev1.Service, error) {
	return c.clientset.CoreV1().Services(service.Namespace).Update(ctx, service, metav1.UpdateOptions{})
}

func (c *Client) DeleteService(ctx context.Context, namespace, name string) error {
	return c.clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}
