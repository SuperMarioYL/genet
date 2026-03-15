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

func TestCreateStatefulSetAllowsSingleReplica(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := models.DefaultConfig()
	handler := NewStatefulSetHandler(
		k8s.NewClientWithClientset(fake.NewSimpleClientset(), config),
		config,
	)

	reqBody := models.StatefulSetRequest{
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
	c.Request = httptest.NewRequest(http.MethodPost, "/api/statefulsets", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("username", "alice")
	c.Set("email", "")

	handler.CreateStatefulSet(c)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d, body=%s", rec.Code, rec.Body.String())
	}

	sts, err := handler.k8sClient.GetClientset().AppsV1().StatefulSets("user-alice").Get(c.Request.Context(), "sts-alice-train", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected statefulset to be created: %v", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 1 {
		t.Fatalf("expected replicas=1, got %#v", sts.Spec.Replicas)
	}
}

func TestBuildStatefulSetResponseSuspended(t *testing.T) {
	config := models.DefaultConfig()
	handler := NewStatefulSetHandler(
		k8s.NewClientWithClientset(fake.NewSimpleClientset(), config),
		config,
	)

	replicas := int32(0)
	suspendedAt := time.Date(2026, 3, 15, 8, 30, 0, 0, time.UTC)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts-alice-train",
			Namespace: "user-alice",
			Annotations: map[string]string{
				"genet.io/image":              "registry.example.com/alice/train:base",
				"genet.io/service-name":       "sts-alice-train-headless",
				"genet.io/suspended":          "true",
				"genet.io/suspended-image":    "registry.example.com/alice/suspend-train:20260315",
				"genet.io/suspended-replicas": "2",
				"genet.io/suspended-at":       suspendedAt.Format(time.RFC3339),
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: "sts-alice-train-headless",
		},
	}

	response := handler.buildStatefulSetResponse(t.Context(), sts)

	if response.Status != "Suspended" {
		t.Fatalf("expected status Suspended, got %q", response.Status)
	}
	if !response.Suspended {
		t.Fatal("expected suspended=true")
	}
	if response.SuspendedImage != "registry.example.com/alice/suspend-train:20260315" {
		t.Fatalf("unexpected suspended image: %q", response.SuspendedImage)
	}
	if response.SuspendedReplicas != 2 {
		t.Fatalf("expected suspended replicas=2, got %d", response.SuspendedReplicas)
	}
	if response.SuspendedAt == nil || !response.SuspendedAt.Equal(suspendedAt) {
		t.Fatalf("unexpected suspended at: %#v", response.SuspendedAt)
	}
}

func TestResumeStatefulSet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := models.DefaultConfig()
	clientset := fake.NewSimpleClientset()
	handler := NewStatefulSetHandler(
		k8s.NewClientWithClientset(clientset, config),
		config,
	)

	replicas := int32(0)
	_, err := clientset.AppsV1().StatefulSets("user-alice").Create(t.Context(), &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sts-alice-train",
			Namespace: "user-alice",
			Labels: map[string]string{
				"genet.io/managed": "true",
			},
			Annotations: map[string]string{
				"genet.io/suspended":          "true",
				"genet.io/suspended-image":    "registry.example.com/alice/suspend-train:20260315",
				"genet.io/suspended-replicas": "2",
			},
		},
		Spec: appsv1.StatefulSetSpec{
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
		t.Fatalf("create statefulset: %v", err)
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/statefulsets/sts-alice-train/resume", nil)
	c.Params = gin.Params{{Key: "id", Value: "sts-alice-train"}}
	c.Set("username", "alice")
	c.Set("email", "")

	handler.ResumeStatefulSet(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	sts, err := clientset.AppsV1().StatefulSets("user-alice").Get(t.Context(), "sts-alice-train", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 2 {
		t.Fatalf("expected replicas restored to 2, got %#v", sts.Spec.Replicas)
	}
	if got := sts.Spec.Template.Spec.Containers[0].Image; got != "registry.example.com/alice/suspend-train:20260315" {
		t.Fatalf("expected restored image, got %q", got)
	}
	if sts.Annotations["genet.io/suspended"] != "false" {
		t.Fatalf("expected suspended=false, got %q", sts.Annotations["genet.io/suspended"])
	}
}

func TestDeleteStatefulSetRejectsUnmanagedStatefulSet(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := models.DefaultConfig()
	clientset := fake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-train",
				Namespace: "user-alice",
			},
		},
	)
	handler := NewStatefulSetHandler(
		k8s.NewClientWithClientset(clientset, config),
		config,
	)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/statefulsets/external-train", nil)
	c.Params = gin.Params{{Key: "id", Value: "external-train"}}
	c.Set("username", "alice")
	c.Set("email", "")

	handler.DeleteStatefulSet(c)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
