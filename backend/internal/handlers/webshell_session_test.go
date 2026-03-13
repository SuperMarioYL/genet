package handlers

import (
	"testing"
	"time"
)

func TestWebShellSessionManagerStoresAndDeletesSessions(t *testing.T) {
	manager := NewWebShellSessionManager(5 * time.Minute)

	session := manager.Create(WebShellSessionSpec{
		PodID:          "pod-alice-dev",
		Namespace:      "user-alice",
		UserIdentifier: "alice",
		Container:      "workspace",
		Shell:          "/bin/sh",
		Cols:           120,
		Rows:           40,
	})

	if session.ID == "" {
		t.Fatalf("expected session id")
	}

	stored, ok := manager.Get(session.ID)
	if !ok {
		t.Fatalf("expected session to be retrievable")
	}
	if stored.PodID != "pod-alice-dev" {
		t.Fatalf("unexpected pod id: %q", stored.PodID)
	}

	manager.Delete(session.ID)
	if _, ok := manager.Get(session.ID); ok {
		t.Fatalf("expected session to be deleted")
	}
}

func TestWebShellSessionManagerExpiresSessions(t *testing.T) {
	manager := NewWebShellSessionManager(20 * time.Millisecond)
	session := manager.Create(WebShellSessionSpec{
		PodID:          "pod-alice-dev",
		Namespace:      "user-alice",
		UserIdentifier: "alice",
		Container:      "workspace",
		Shell:          "/bin/sh",
		Cols:           120,
		Rows:           40,
	})

	time.Sleep(50 * time.Millisecond)

	if _, ok := manager.Get(session.ID); ok {
		t.Fatalf("expected expired session to be removed")
	}
}
