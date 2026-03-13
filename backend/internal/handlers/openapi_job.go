package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/models"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func buildOpenAPIJobResponse(job *batchv1.Job) models.OpenAPIJobResponse {
	response := models.OpenAPIJobResponse{
		Name:        job.Name,
		Namespace:   job.Namespace,
		Parallelism: job.Spec.Parallelism,
		Completions: job.Spec.Completions,
		Active:      job.Status.Active,
		Succeeded:   job.Status.Succeeded,
		Failed:      job.Status.Failed,
		Status:      openAPIJobStatus(job),
		CreatedAt:   job.CreationTimestamp.Time,
	}

	if len(job.Spec.Template.Spec.Containers) > 0 {
		response.Image = job.Spec.Template.Spec.Containers[0].Image
	}
	if job.Status.CompletionTime != nil {
		completionTime := job.Status.CompletionTime.Time
		response.CompletionTime = &completionTime
	}

	return response
}

func openAPIJobStatus(job *batchv1.Job) string {
	switch {
	case job.Status.Succeeded > 0:
		return "Succeeded"
	case job.Status.Failed > 0:
		return "Failed"
	case job.Status.Active > 0:
		return "Running"
	default:
		return "Pending"
	}
}

func (h *OpenAPIHandler) CreateJob(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	var req models.OpenAPIJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的请求参数: %v", err)})
		return
	}
	if err := ValidateOpenAPIJobRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	ctx := c.Request.Context()
	if err := h.k8sClient.EnsureNamespace(ctx, namespace); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to ensure namespace: %v", err)})
		return
	}

	job, err := h.k8sClient.BuildJobFromOpenAPIRequest(ctx, namespace, ownerUser, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.k8sClient.CreateJob(ctx, job)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("job %q already exists", req.Name)})
			return
		}
		h.log.Error("Failed to create job", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create job: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, buildOpenAPIJobResponse(created))
}

func (h *OpenAPIHandler) ListJobs(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	jobs, err := h.k8sClient.ListJobs(c.Request.Context(), namespace, openAPIOwnerLabelSelector(ownerUser, c.Query("labelSelector")))
	if err != nil {
		h.log.Error("Failed to list jobs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list jobs: %v", err)})
		return
	}

	resp := models.OpenAPIJobListResponse{
		Jobs: make([]models.OpenAPIJobResponse, 0, len(jobs.Items)),
	}
	for i := range jobs.Items {
		resp.Jobs = append(resp.Jobs, buildOpenAPIJobResponse(&jobs.Items[i]))
	}

	c.JSON(http.StatusOK, resp)
}

func (h *OpenAPIHandler) GetJob(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	job, err := h.k8sClient.GetJob(c.Request.Context(), openAPIOwnerNamespace(ownerUser), c.Param("name"))
	if err != nil || !isOpenAPIOwnedBy(job.Labels, ownerUser) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	c.JSON(http.StatusOK, buildOpenAPIJobResponse(job))
}

func (h *OpenAPIHandler) UpdateJob(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	var req models.OpenAPIJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("无效的请求参数: %v", err)})
		return
	}
	req.Name = c.Param("name")
	if err := ValidateOpenAPIJobRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	ctx := c.Request.Context()
	existing, err := h.k8sClient.GetJob(ctx, namespace, req.Name)
	if err != nil || !isOpenAPIOwnedBy(existing.Labels, ownerUser) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if existing.Status.Active > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "running job cannot be updated"})
		return
	}

	job, err := h.k8sClient.BuildJobFromOpenAPIRequest(ctx, namespace, ownerUser, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.k8sClient.DeleteJob(ctx, namespace, req.Name); err != nil {
		h.log.Error("Failed to delete job before recreate", zap.String("name", req.Name), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete existing job: %v", err)})
		return
	}

	created, err := h.k8sClient.CreateJob(ctx, job)
	if err != nil {
		h.log.Error("Failed to recreate job", zap.String("name", req.Name), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to recreate job: %v", err)})
		return
	}

	c.JSON(http.StatusOK, buildOpenAPIJobResponse(created))
}

func (h *OpenAPIHandler) DeleteJob(c *gin.Context) {
	ownerUser := c.GetString("openapiOwnerUser")
	if _, ok := applyOpenAPIOwnerUserContext(c); !ok {
		return
	}

	namespace := openAPIOwnerNamespace(ownerUser)
	name := c.Param("name")
	job, err := h.k8sClient.GetJob(c.Request.Context(), namespace, name)
	if err != nil || !isOpenAPIOwnedBy(job.Labels, ownerUser) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	if err := h.k8sClient.DeleteJob(c.Request.Context(), namespace, name); err != nil {
		h.log.Error("Failed to delete job", zap.String("name", name), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete job: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "job deleted"})
}
