package genetcli

import (
	"strings"
	"testing"
)

func TestFormatOutputReturnsJSONWhenRequested(t *testing.T) {
	out, err := formatOutput(true, map[string]string{"username": "alice"})
	if err != nil {
		t.Fatalf("formatOutput: %v", err)
	}
	if !strings.Contains(out, "\"username\": \"alice\"") {
		t.Fatalf("expected json output, got %s", out)
	}
}

func TestFormatOutputReturnsPlainTextWhenRequested(t *testing.T) {
	out, err := formatOutput(false, map[string]string{"username": "alice"})
	if err != nil {
		t.Fatalf("formatOutput: %v", err)
	}
	if !strings.Contains(out, "username: alice") {
		t.Fatalf("expected plain output, got %s", out)
	}
}
