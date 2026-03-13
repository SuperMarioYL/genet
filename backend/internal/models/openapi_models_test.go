package models

import "testing"

func TestOpenAPIServiceRequestValidationTagsExist(t *testing.T) {
	req := OpenAPIServiceRequest{}
	if req.Name != "" {
		t.Fatal("expected zero value request")
	}
}

func TestOpenAPIConfigMapRequestZeroValue(t *testing.T) {
	req := OpenAPIConfigMapRequest{}
	if req.Name != "" {
		t.Fatal("expected zero value request")
	}
}

func TestOpenAPIJobRequestZeroValue(t *testing.T) {
	req := OpenAPIJobRequest{}
	if req.Name != "" {
		t.Fatal("expected zero value request")
	}
}
