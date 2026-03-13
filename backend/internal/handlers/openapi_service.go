package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

func openAPIOwnerNamespace(ownerUser string) string {
	userIdentifier := k8s.GetUserIdentifier(ownerUser, "")
	return k8s.GetNamespaceForUserIdentifier(userIdentifier)
}

func buildOpenAPIServiceResponse(service *corev1.Service) models.OpenAPIServiceResponse {
	resp := models.OpenAPIServiceResponse{
		Name:        service.Name,
		Namespace:   service.Namespace,
		Type:        string(service.Spec.Type),
		Selector:    service.Spec.Selector,
		ClusterIP:   service.Spec.ClusterIP,
		ExternalIPs: append([]string{}, service.Spec.ExternalIPs...),
		Ports:       make([]models.OpenAPIServicePort, 0, len(service.Spec.Ports)),
		CreatedAt:   service.CreationTimestamp.Time,
	}

	for _, port := range service.Spec.Ports {
		targetPort := ""
		if port.TargetPort.IntVal != 0 || port.TargetPort.StrVal != "" {
			targetPort = port.TargetPort.String()
		}
		resp.Ports = append(resp.Ports, models.OpenAPIServicePort{
			Name:       port.Name,
			Protocol:   string(port.Protocol),
			Port:       port.Port,
			TargetPort: targetPort,
			NodePort:   port.NodePort,
		})
	}

	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			resp.LoadBalancer = append(resp.LoadBalancer, ingress.IP)
		}
		if ingress.Hostname != "" {
			resp.LoadBalancer = append(resp.LoadBalancer, ingress.Hostname)
		}
	}

	return resp
}

func (h *OpenAPIHandler) CreateService(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	var req models.OpenAPIServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的请求参数: %v", err)})
		return
	}
	if err := ValidateOpenAPIServiceRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	ctx := c.Request.Context()

	if err := h.k8sClient.EnsureNamespace(ctx, namespace); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to ensure namespace: %v", err)})
		return
	}

	if req.TargetPodName != "" {
		if _, err := h.k8sClient.GetPod(ctx, namespace, req.TargetPodName); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "target pod not found"})
			return
		}
	}

	service, err := k8s.BuildServiceFromOpenAPIRequest(namespace, ownerUser, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.k8sClient.CreateService(ctx, service)
	if err != nil {
		h.log.Error("Failed to create service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create service: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, buildOpenAPIServiceResponse(created))
}

func (h *OpenAPIHandler) ListServices(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	labelSelector := c.Query("labelSelector")
	services, err := h.k8sClient.ListServices(c.Request.Context(), namespace, openAPIOwnerLabelSelector(ownerUser, labelSelector))
	if err != nil {
		h.log.Error("Failed to list services", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list services: %v", err)})
		return
	}

	resp := models.OpenAPIServiceListResponse{
		Services: make([]models.OpenAPIServiceResponse, 0, len(services.Items)),
	}
	for i := range services.Items {
		resp.Services = append(resp.Services, buildOpenAPIServiceResponse(&services.Items[i]))
	}

	c.JSON(http.StatusOK, resp)
}

func (h *OpenAPIHandler) GetService(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	service, err := h.k8sClient.GetService(c.Request.Context(), openAPIOwnerNamespace(ownerUser), c.Param("name"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
		return
	}
	if !isOpenAPIOwnedBy(service.Labels, ownerUser) {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
		return
	}

	c.JSON(http.StatusOK, buildOpenAPIServiceResponse(service))
}

func (h *OpenAPIHandler) UpdateService(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	var req models.OpenAPIServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的请求参数: %v", err)})
		return
	}
	req.Name = c.Param("name")
	if err := ValidateOpenAPIServiceRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	ctx := c.Request.Context()
	existing, err := h.k8sClient.GetService(ctx, namespace, req.Name)
	if err != nil || !isOpenAPIOwnedBy(existing.Labels, ownerUser) {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
		return
	}

	service, err := k8s.BuildServiceFromOpenAPIRequest(namespace, ownerUser, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	service.ResourceVersion = existing.ResourceVersion
	service.Spec.ClusterIP = existing.Spec.ClusterIP
	service.Spec.ClusterIPs = existing.Spec.ClusterIPs
	service.Spec.HealthCheckNodePort = existing.Spec.HealthCheckNodePort
	service.Spec.IPFamilies = existing.Spec.IPFamilies
	service.Spec.IPFamilyPolicy = existing.Spec.IPFamilyPolicy
	service.Spec.InternalTrafficPolicy = existing.Spec.InternalTrafficPolicy
	service.ObjectMeta.CreationTimestamp = existing.CreationTimestamp

	updated, err := h.k8sClient.UpdateService(ctx, service)
	if err != nil {
		h.log.Error("Failed to update service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update service: %v", err)})
		return
	}

	c.JSON(http.StatusOK, buildOpenAPIServiceResponse(updated))
}

func (h *OpenAPIHandler) DeleteService(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	name := c.Param("name")
	service, err := h.k8sClient.GetService(c.Request.Context(), namespace, name)
	if err != nil || !isOpenAPIOwnedBy(service.Labels, ownerUser) {
		c.JSON(http.StatusNotFound, gin.H{"error": "service not found"})
		return
	}

	if err := h.k8sClient.DeleteService(c.Request.Context(), namespace, name); err != nil {
		h.log.Error("Failed to delete service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete service: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "service deleted"})
}
