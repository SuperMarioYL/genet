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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestOpenAPIServiceCreateUsesOwnerNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ownerUser := "alice"
	userIdentifier := k8s.GetUserIdentifier(ownerUser, "")
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)

	clientset := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-alice-train",
			Namespace: namespace,
			Labels:    map[string]string{"app": "pod-alice-train"},
		},
	})

	k8sClient := k8s.NewClientWithClientset(clientset, &models.Config{})
	handler := NewOpenAPIHandler(k8sClient, &models.Config{})

	reqBody := models.OpenAPIServiceRequest{
		Name:          "svc-demo",
		Type:          "ClusterIP",
		TargetPodName: "pod-alice-train",
		Ports: []models.OpenAPIServicePort{
			{Name: "http", Port: 80, TargetPort: "8080"},
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/open/services", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("openapiOwnerUser", ownerUser)

	handler.CreateService(c)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	service, err := clientset.CoreV1().Services(namespace).Get(c.Request.Context(), "svc-demo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected service to be created: %v", err)
	}
	if service.Namespace != namespace {
		t.Fatalf("expected namespace %s, got %s", namespace, service.Namespace)
	}
}
