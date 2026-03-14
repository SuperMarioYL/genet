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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestOpenAPIStatefulSetCreateUsesOwnerNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ownerUser := "alice"
	userIdentifier := k8s.GetUserIdentifier(ownerUser, "")
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)

	handler := NewOpenAPIHandler(
		k8s.NewClientWithClientset(fake.NewSimpleClientset(), models.DefaultConfig()),
		models.DefaultConfig(),
	)

	reqBody := models.OpenAPIStatefulSetCreateRequest{
		Image:    "busybox:latest",
		GPUCount: 0,
		Replicas: 2,
		Name:     "train",
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/open/statefulsets", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("openapiOwnerUser", ownerUser)

	handler.CreateStatefulSet(c)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	sts, err := handler.k8sClient.GetClientset().AppsV1().StatefulSets(namespace).Get(c.Request.Context(), "sts-alice-train", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected statefulset to be created: %v", err)
	}
	if sts.Namespace != namespace {
		t.Fatalf("expected namespace %s, got %s", namespace, sts.Namespace)
	}
}
