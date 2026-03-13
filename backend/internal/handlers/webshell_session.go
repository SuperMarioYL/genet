package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	defaultWebShellCols = 120
	defaultWebShellRows = 40
)

type WebShellSessionSpec struct {
	PodID          string
	Namespace      string
	UserIdentifier string
	Container      string
	Shell          string
	Cols           int
	Rows           int
}

type WebShellSession struct {
	ID             string    `json:"sessionId"`
	PodID          string    `json:"podId"`
	Namespace      string    `json:"namespace"`
	UserIdentifier string    `json:"userIdentifier"`
	Container      string    `json:"container"`
	Shell          string    `json:"shell"`
	Cols           int       `json:"cols"`
	Rows           int       `json:"rows"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type WebShellSessionResponse struct {
	SessionID    string    `json:"sessionId"`
	WebSocketURL string    `json:"webSocketURL"`
	Container    string    `json:"container"`
	Shell        string    `json:"shell"`
	Cols         int       `json:"cols"`
	Rows         int       `json:"rows"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type WebShellSessionManager struct {
	ttl      time.Duration
	mu       sync.Mutex
	sessions map[string]WebShellSession
}

func NewWebShellSessionManager(ttl time.Duration) *WebShellSessionManager {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &WebShellSessionManager{
		ttl:      ttl,
		sessions: make(map[string]WebShellSession),
	}
}

func (m *WebShellSessionManager) Create(spec WebShellSessionSpec) WebShellSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneLocked(time.Now())

	now := time.Now()
	session := WebShellSession{
		ID:             newWebShellSessionID(),
		PodID:          spec.PodID,
		Namespace:      spec.Namespace,
		UserIdentifier: spec.UserIdentifier,
		Container:      spec.Container,
		Shell:          spec.Shell,
		Cols:           normalizeWebShellCols(spec.Cols),
		Rows:           normalizeWebShellRows(spec.Rows),
		CreatedAt:      now,
		ExpiresAt:      now.Add(m.ttl),
	}
	m.sessions[session.ID] = session
	return session
}

func (m *WebShellSessionManager) Get(id string) (WebShellSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneLocked(time.Now())
	session, ok := m.sessions[id]
	return session, ok
}

func (m *WebShellSessionManager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, id)
}

func (m *WebShellSessionManager) pruneLocked(now time.Time) {
	for id, session := range m.sessions {
		if !session.ExpiresAt.After(now) {
			delete(m.sessions, id)
		}
	}
}

func normalizeWebShellCols(cols int) int {
	if cols <= 0 {
		return defaultWebShellCols
	}
	return cols
}

func normalizeWebShellRows(rows int) int {
	if rows <= 0 {
		return defaultWebShellRows
	}
	return rows
}

func newWebShellSessionID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(raw[:])
}
