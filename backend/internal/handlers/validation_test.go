package handlers

import (
	"testing"

	"github.com/uc-package/genet/internal/models"
)

func TestValidateOpenAPIServiceRequestRejectsMissingSelector(t *testing.T) {
	req := models.OpenAPIServiceRequest{
		Name:  "svc-demo",
		Type:  "ClusterIP",
		Ports: []models.OpenAPIServicePort{{Port: 80, TargetPort: "8080"}},
	}

	err := ValidateOpenAPIServiceRequest(&req)
	if err == nil {
		t.Fatal("expected selector validation error")
	}
}

func TestValidateOpenAPIConfigMapRequestRejectsInvalidBinaryData(t *testing.T) {
	req := models.OpenAPIConfigMapRequest{
		Name:       "cm-demo",
		BinaryData: map[string]string{"bad.bin": "%%%"},
	}

	err := ValidateOpenAPIConfigMapRequest(&req)
	if err == nil {
		t.Fatal("expected binaryData validation error")
	}
}

func TestValidateOpenAPIJobRequestRejectsInvalidRestartPolicy(t *testing.T) {
	req := models.OpenAPIJobRequest{
		Name:          "job-demo",
		Image:         "busybox:latest",
		RestartPolicy: "Always",
	}

	err := ValidateOpenAPIJobRequest(&req)
	if err == nil {
		t.Fatal("expected restartPolicy validation error")
	}
}
