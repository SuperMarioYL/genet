package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	"github.com/uc-package/genet/internal/models"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAdminMe_ForbiddenForNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.AdminUsers = []string{"alice"}
	auth.InitAuthMiddleware(cfg)

	h := NewAdminHandler(cfg, nil)
	r := gin.New()
	r.GET("/api/admin/me", auth.AuthMiddleware(cfg), auth.RequireAdmin(cfg), h.GetMe)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	req.Header.Set("X-Auth-Request-User", "bob")
	req.Header.Set("X-Auth-Request-Email", "bob@example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestAdminMe_OKForAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.AdminUsers = []string{"alice"}
	auth.InitAuthMiddleware(cfg)

	h := NewAdminHandler(cfg, nil)
	r := gin.New()
	r.GET("/api/admin/me", auth.AuthMiddleware(cfg), auth.RequireAdmin(cfg), h.GetMe)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	req.Header.Set("X-Auth-Request-User", "alice")
	req.Header.Set("X-Auth-Request-Email", "alice@example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestAdminListNodePools_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := adminTestConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-cpu-only",
				Labels: map[string]string{},
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.9"}},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-shared",
				Labels: map[string]string{},
			},
			Status: corev1.NodeStatus{
				Capacity:    corev1.ResourceList{corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("8")},
				Allocatable: corev1.ResourceList{corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("8")},
				Addresses:   []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.1"}},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-exclusive",
				Labels: map[string]string{"genet.io/node-pool": "non-shared"},
			},
			Status: corev1.NodeStatus{
				Capacity:    corev1.ResourceList{corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("8")},
				Allocatable: corev1.ResourceList{corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("8")},
				Addresses:   []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.2"}},
			},
		},
	)

	rec := performAdminRequest(t, cfg, k8s.NewClientForTest(clientset, cfg), http.MethodGet, "/api/admin/nodes/pools", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		Nodes []struct {
			NodeName string `json:"nodeName"`
			PoolType string `json:"poolType"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(resp.Nodes))
	}
	for _, node := range resp.Nodes {
		if node.NodeName == "node-cpu-only" {
			t.Fatal("expected cpu-only node to be excluded from node pools")
		}
	}
}

func TestAdminUpdateNodePool_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := adminTestConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-a",
				Labels: map[string]string{},
			},
		},
	)

	rec := performAdminRequest(t, cfg, k8s.NewClientForTest(clientset, cfg), http.MethodPatch, "/api/admin/nodes/node-a/pool", map[string]string{
		"poolType": "exclusive",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	node, err := clientset.CoreV1().Nodes().Get(t.Context(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get updated node: %v", err)
	}
	if node.Labels["genet.io/node-pool"] != "non-shared" {
		t.Fatalf("expected exclusive label to be set, got %#v", node.Labels)
	}
}

func TestAdminListUserPools_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := adminTestConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.OpenAPI.Namespace}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "user-empty"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-bob-dev",
				Namespace: "user-bob",
				Labels: map[string]string{
					"genet.io/user": "bob",
				},
				Annotations: map[string]string{
					"genet.io/email": "bob@example.com",
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deploy-carol-dev",
				Namespace: "user-carol",
				Labels: map[string]string{
					"genet.io/user": "carol",
				},
				Annotations: map[string]string{
					"genet.io/email": "carol@example.com",
				},
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "sts-dave-train",
				Namespace: "user-dave",
				Labels: map[string]string{
					"genet.io/user": "dave",
				},
				Annotations: map[string]string{
					"genet.io/email": "dave@example.com",
				},
			},
		},
	)
	client := k8s.NewClientForTest(clientset, cfg)
	if err := client.UpsertUserPoolBinding(t.Context(), k8s.UserPoolBindingRecord{
		Username:  "alice",
		PoolType:  k8s.UserPoolTypeExclusive,
		UpdatedAt: metav1.Now().UTC(),
		UpdatedBy: "admin",
	}); err != nil {
		t.Fatalf("failed to seed user pool binding: %v", err)
	}

	rec := performAdminRequest(t, cfg, client, http.MethodGet, "/api/admin/users/pools", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Users []struct {
			Username string `json:"username"`
			PoolType string `json:"poolType"`
		} `json:"users"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Users) != 4 {
		t.Fatalf("expected 4 users, got %d: %#v", len(resp.Users), resp.Users)
	}

	usernames := map[string]bool{}
	for _, user := range resp.Users {
		usernames[user.Username] = true
	}
	for _, username := range []string{"alice", "bob", "carol", "dave"} {
		if !usernames[username] {
			t.Fatalf("expected user %q to be present, got %#v", username, resp.Users)
		}
	}
	if usernames["empty"] {
		t.Fatalf("expected namespace-only user to be excluded, got %#v", resp.Users)
	}
}

func TestAdminUpdateUserPool_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := adminTestConfig()
	clientset := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.OpenAPI.Namespace}})
	client := k8s.NewClientForTest(clientset, cfg)

	rec := performAdminRequest(t, cfg, client, http.MethodPatch, "/api/admin/users/alice/pool", map[string]string{
		"poolType": "exclusive",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	record, ok, err := client.GetUserPoolBinding(t.Context(), "alice")
	if err != nil {
		t.Fatalf("GetUserPoolBinding returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected binding to exist")
	}
	if record.PoolType != k8s.UserPoolTypeExclusive {
		t.Fatalf("expected pool type exclusive, got %s", record.PoolType)
	}
}

func TestAdminDeleteUser_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := adminTestConfig()
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.OpenAPI.Namespace}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "user-alice"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-alice-dev", Namespace: "user-alice"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy-alice-dev", Namespace: "user-alice"}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "sts-alice-train", Namespace: "user-alice"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-alice-data", Namespace: "user-alice"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: k8s.UserImagesConfigMapName, Namespace: "user-alice"}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "user-alice-role", Namespace: "user-alice"}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "user-alice-binding", Namespace: "user-alice"}},
	)
	client := k8s.NewClientForTest(clientset, cfg)
	if err := client.UpsertUserPoolBinding(t.Context(), k8s.UserPoolBindingRecord{
		Username:  "alice",
		PoolType:  k8s.UserPoolTypeExclusive,
		UpdatedAt: metav1.Now().UTC(),
		UpdatedBy: "admin",
	}); err != nil {
		t.Fatalf("failed to seed user pool binding: %v", err)
	}

	rec := performAdminRequest(t, cfg, client, http.MethodDelete, "/api/admin/users/alice", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if _, err := clientset.CoreV1().Namespaces().Get(t.Context(), "user-alice", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected namespace to be deleted, got err=%v", err)
	}
	if _, err := clientset.CoreV1().Pods("user-alice").Get(t.Context(), "pod-alice-dev", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected pod to be deleted, got err=%v", err)
	}
	if _, err := clientset.AppsV1().Deployments("user-alice").Get(t.Context(), "deploy-alice-dev", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected deployment to be deleted, got err=%v", err)
	}
	if _, err := clientset.AppsV1().StatefulSets("user-alice").Get(t.Context(), "sts-alice-train", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected statefulset to be deleted, got err=%v", err)
	}
	if _, err := clientset.CoreV1().PersistentVolumeClaims("user-alice").Get(t.Context(), "pvc-alice-data", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected pvc to be deleted, got err=%v", err)
	}
	if _, err := clientset.RbacV1().Roles("user-alice").Get(t.Context(), "user-alice-role", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected role to be deleted, got err=%v", err)
	}
	if _, err := clientset.RbacV1().RoleBindings("user-alice").Get(t.Context(), "user-alice-binding", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected rolebinding to be deleted, got err=%v", err)
	}

	if _, ok, err := client.GetUserPoolBinding(t.Context(), "alice"); err != nil {
		t.Fatalf("GetUserPoolBinding returned error: %v", err)
	} else if ok {
		t.Fatal("expected user pool binding to be deleted")
	}
}

func adminTestConfig() *models.Config {
	cfg := models.DefaultConfig()
	cfg.OAuth.Enabled = true
	cfg.AdminUsers = []string{"alice"}
	return cfg
}

func performAdminRequest(t *testing.T, cfg *models.Config, client *k8s.Client, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()

	auth.InitAuthMiddleware(cfg)
	h := NewAdminHandler(cfg, client)
	r := gin.New()
	admin := r.Group("/api/admin")
	admin.Use(auth.AuthMiddleware(cfg), auth.RequireAdmin(cfg))
	admin.GET("/me", h.GetMe)
	admin.GET("/nodes/pools", h.ListNodePools)
	admin.PATCH("/nodes/:name/pool", h.UpdateNodePool)
	admin.GET("/users/pools", h.ListUserPools)
	admin.PATCH("/users/:username/pool", h.UpdateUserPool)
	admin.DELETE("/users/:username", h.DeleteUser)

	var reqBody *bytes.Reader
	if body == nil {
		reqBody = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to encode request body: %v", err)
		}
		reqBody = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Request-User", "alice")
	req.Header.Set("X-Auth-Request-Email", "alice@example.com")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}
