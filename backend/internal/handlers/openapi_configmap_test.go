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

func TestOpenAPIConfigMapUpdateRejectsImmutableConfigMap(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ownerUser := "alice"
	userIdentifier := k8s.GetUserIdentifier(ownerUser, "")
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)
	immutable := true

	clientset := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cm-demo",
			Namespace: namespace,
			Labels: map[string]string{
				"genet.io/openapi-owner": ownerUser,
			},
		},
		Immutable: &immutable,
		Data:      map[string]string{"key": "old"},
	})

	handler := NewOpenAPIHandler(k8s.NewClientWithClientset(clientset, &models.Config{}), &models.Config{})
	reqBody := models.OpenAPIConfigMapRequest{
		Name: "cm-demo",
		Data: map[string]string{"key": "new"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "name", Value: "cm-demo"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/open/configmaps/cm-demo", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("openapiOwnerUser", ownerUser)

	handler.UpdateConfigMap(c)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
