package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestOpenAPIJobCreateAcceptsSimplifiedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ownerUser := "alice"
	userIdentifier := k8s.GetUserIdentifier(ownerUser, "")
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	clientset := fake.NewSimpleClientset()

	handler := NewOpenAPIHandler(k8s.NewClientWithClientset(clientset, &models.Config{
		Pod: models.PodConfig{
			StartupScript: "echo ready",
		},
	}), &models.Config{
		Pod: models.PodConfig{
			StartupScript: "echo ready",
		},
	})

	reqBody := models.OpenAPIJobRequest{
		Name:    "job-demo",
		Image:   "busybox:latest",
		Command: []string{"sh"},
		Args:    []string{"-c", "echo hi"},
		CPU:     "2",
		Memory:  "4Gi",
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/open/jobs", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("openapiOwnerUser", ownerUser)

	handler.CreateJob(c)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	job, err := clientset.BatchV1().Jobs(namespace).Get(c.Request.Context(), "job-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected job to be created: %v", err)
	}
	if job.Namespace != namespace {
		t.Fatalf("expected namespace %s, got %s", namespace, job.Namespace)
	}
	if got := job.Spec.Template.Spec.Containers[0].Image; got != "busybox:latest" {
		t.Fatalf("expected image busybox:latest, got %s", got)
	}
}

func TestOpenAPIJobUpdateRejectsActiveJob(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ownerUser := "alice"
	userIdentifier := k8s.GetUserIdentifier(ownerUser, "")
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	clientset := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-demo",
			Namespace: namespace,
			Labels: map[string]string{
				"genet.io/openapi-owner": ownerUser,
			},
		},
		Status: batchv1.JobStatus{
			Active: 1,
		},
	})

	handler := NewOpenAPIHandler(k8s.NewClientWithClientset(clientset, &models.Config{
		Pod: models.PodConfig{
			StartupScript: "echo ready",
		},
	}), &models.Config{
		Pod: models.PodConfig{
			StartupScript: "echo ready",
		},
	})

	reqBody := models.OpenAPIJobRequest{
		Name:  "job-demo",
		Image: "busybox:latest",
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "name", Value: "job-demo"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/open/jobs/job-demo", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("openapiOwnerUser", ownerUser)

	handler.UpdateJob(c)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
