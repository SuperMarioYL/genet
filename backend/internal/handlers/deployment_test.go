package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCreateDeploymentAllowsSingleReplica(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := models.DefaultConfig()
	handler := NewDeploymentHandler(
		k8s.NewClientWithClientset(fake.NewSimpleClientset(), config),
		config,
	)

	reqBody := models.DeploymentRequest{
		Image:    "busybox:latest",
		GPUCount: 0,
		Replicas: 1,
		Name:     "train",
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/deployments", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("username", "alice")
	c.Set("email", "")

	handler.CreateDeployment(c)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	deploy, err := handler.k8sClient.GetClientset().AppsV1().Deployments("user-alice").Get(c.Request.Context(), "deploy-alice-train", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected deployment to be created: %v", err)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 1 {
		t.Fatalf("expected replicas=1, got %#v", deploy.Spec.Replicas)
	}
}

func TestBuildDeploymentResponseSuspended(t *testing.T) {
	config := models.DefaultConfig()
	handler := NewDeploymentHandler(
		k8s.NewClientWithClientset(fake.NewSimpleClientset(), config),
		config,
	)

	replicas := int32(0)
	suspendedAt := time.Date(2026, 3, 15, 8, 30, 0, 0, time.UTC)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deploy-alice-train",
			Namespace: "user-alice",
			Annotations: map[string]string{
				"genet.io/image":              "registry.example.com/alice/train:base",
				"genet.io/suspended":          "true",
				"genet.io/suspended-image":    "registry.example.com/alice/suspend-train:20260315",
				"genet.io/suspended-replicas": "3",
				"genet.io/suspended-at":       suspendedAt.Format(time.RFC3339),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}

	response := handler.buildDeploymentResponse(t.Context(), deploy)

	if response.Status != "Suspended" {
		t.Fatalf("expected status Suspended, got %q", response.Status)
	}
	if !response.Suspended {
		t.Fatal("expected suspended=true")
	}
	if response.SuspendedImage != "registry.example.com/alice/suspend-train:20260315" {
		t.Fatalf("unexpected suspended image: %q", response.SuspendedImage)
	}
	if response.SuspendedReplicas != 3 {
		t.Fatalf("expected suspended replicas=3, got %d", response.SuspendedReplicas)
	}
	if response.SuspendedAt == nil || !response.SuspendedAt.Equal(suspendedAt) {
		t.Fatalf("unexpected suspended at: %#v", response.SuspendedAt)
	}
}

func TestResumeDeployment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := models.DefaultConfig()
	clientset := fake.NewSimpleClientset()
	handler := NewDeploymentHandler(
		k8s.NewClientWithClientset(clientset, config),
		config,
	)

	replicas := int32(0)
	_, err := clientset.AppsV1().Deployments("user-alice").Create(t.Context(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deploy-alice-train",
			Namespace: "user-alice",
			Annotations: map[string]string{
				"genet.io/suspended":          "true",
				"genet.io/suspended-image":    "registry.example.com/alice/suspend-train:20260315",
				"genet.io/suspended-replicas": "3",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "workspace", Image: "registry.example.com/alice/train:base"},
					},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/deployments/deploy-alice-train/resume", nil)
	c.Params = gin.Params{{Key: "id", Value: "deploy-alice-train"}}
	c.Set("username", "alice")
	c.Set("email", "")

	handler.ResumeDeployment(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	deploy, err := clientset.AppsV1().Deployments("user-alice").Get(t.Context(), "deploy-alice-train", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != 3 {
		t.Fatalf("expected replicas restored to 3, got %#v", deploy.Spec.Replicas)
	}
	if got := deploy.Spec.Template.Spec.Containers[0].Image; got != "registry.example.com/alice/suspend-train:20260315" {
		t.Fatalf("expected restored image, got %q", got)
	}
	if deploy.Annotations["genet.io/suspended"] != "false" {
		t.Fatalf("expected suspended=false, got %q", deploy.Annotations["genet.io/suspended"])
	}
}
