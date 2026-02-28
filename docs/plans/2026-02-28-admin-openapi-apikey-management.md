# Admin OpenAPI Key Management Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an admin-only management page (OAuth login still used) where configured admins can create/update/revoke OpenAPI API keys and bind each key to a user identity that can be resolved during OpenAPI auth.

**Architecture:** Keep current OAuth session auth for UI/API, add an admin allowlist from Helm config, and add dedicated `/api/admin/*` APIs guarded by `AuthMiddleware + RequireAdmin`. Persist managed API keys as hashed records in Kubernetes (Secret), and make OpenAPI middleware resolve key metadata (owner user + scope) from that store.

**Tech Stack:** Go (Gin, client-go), React + Ant Design, Helm chart config injection, Kubernetes Secret-backed metadata.

---

## Scope And Decisions

1. Admin identity source: OAuth `username` first, fallback `email`.
2. Admin allowlist: `backend.config.adminUsers` from Helm.
3. Managed API key storage: Kubernetes Secret JSON payload (single source of truth for admin-created keys).
4. API key owner mapping: admin assigns `ownerUser` in management page; OpenAPI middleware resolves it from the key.
5. Backward compatibility: keep existing `openAPI.apiKeys` as legacy read path during migration (optional flag in code comments).

## API Contracts (Target)

1. `GET /api/admin/me`
   - Response: `{ "username": "...", "email": "...", "isAdmin": true }`
2. `GET /api/admin/apikeys`
   - Response: `{ "items": [{ "id": "...", "name": "...", "ownerUser": "...", "scope": "read|write", "enabled": true, "keyPreview": "gk_xxx...", "createdAt": "...", "updatedAt": "...", "createdBy": "..." }] }`
3. `POST /api/admin/apikeys`
   - Request: `{ "name": "ci-bot", "ownerUser": "alice", "scope": "write", "expiresAt": "2026-12-31T00:00:00Z" }`
   - Response: `{ "item": { ...metadata... }, "plaintextKey": "gk_xxx" }` (only returned once)
4. `PATCH /api/admin/apikeys/:id`
   - Request: `{ "name"?: "...", "ownerUser"?: "...", "scope"?: "read|write", "enabled"?: true, "expiresAt"?: "..." }`
   - Response: `{ "item": { ...updated... } }`
5. `DELETE /api/admin/apikeys/:id`
   - Response: `{ "message": "deleted" }`

## Data Model (Target)

```go
type APIKeyRecord struct {
    ID         string     `json:"id"`
    Name       string     `json:"name"`
    OwnerUser  string     `json:"ownerUser"`
    Scope      string     `json:"scope"` // read|write
    Enabled    bool       `json:"enabled"`
    KeyHash    string     `json:"keyHash"` // sha256 hex
    KeyPreview string     `json:"keyPreview"`
    ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
    CreatedAt  time.Time  `json:"createdAt"`
    UpdatedAt  time.Time  `json:"updatedAt"`
    CreatedBy  string     `json:"createdBy"`
}
```

---

### Task 1: Config And Helm Wiring

**Files:**
- Modify: `backend/internal/models/config.go`
- Modify: `helm/genet/values.yaml`
- Modify: `helm/genet/templates/configmap-config.yaml`
- Test: `backend/internal/models/config_admin_test.go` (new)

**Step 1: Write the failing test**

```go
func TestLoadConfig_WithAdminUsersAndOpenAPI(t *testing.T) {
    raw := []byte(`
adminUsers: ["alice", "bob@example.com"]
openAPI:
  enabled: true
  namespace: genet-open-api
  apiKeys: ["legacy-key"]
`)
    var cfg Config
    if err := yaml.Unmarshal(raw, &cfg); err != nil { t.Fatal(err) }
    if len(cfg.AdminUsers) != 2 { t.Fatalf("want 2 admin users, got %d", len(cfg.AdminUsers)) }
    if !cfg.OpenAPI.Enabled { t.Fatal("openapi not loaded") }
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/models -run TestLoadConfig_WithAdminUsersAndOpenAPI -v`  
Expected: FAIL (`cfg.AdminUsers` missing field).

**Step 3: Write minimal implementation**

- Add `AdminUsers []string` to `Config`.
- Add defaults in `DefaultConfig`.
- In Helm values add:
  - `backend.config.adminUsers: []`
  - `backend.config.openAPI` block (`enabled`, `namespace`, `apiKeys`).
- In ConfigMap template render `adminUsers` and `openAPI`.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/models -run TestLoadConfig_WithAdminUsersAndOpenAPI -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/models/config.go backend/internal/models/config_admin_test.go helm/genet/values.yaml helm/genet/templates/configmap-config.yaml
git commit -m "feat(config): add adminUsers and helm wiring for openapi"
```

### Task 2: Admin Authorization Middleware

**Files:**
- Create: `backend/internal/auth/admin.go`
- Test: `backend/internal/auth/admin_test.go`

**Step 1: Write the failing test**

```go
func TestIsAdmin_MatchesUsernameOrEmail(t *testing.T) {
    cfg := &models.Config{AdminUsers: []string{"alice", "bob@example.com"}}
    if !IsAdmin(cfg, "alice", "alice@example.com") { t.Fatal("username should match") }
    if !IsAdmin(cfg, "bob", "bob@example.com") { t.Fatal("email should match") }
    if IsAdmin(cfg, "charlie", "charlie@example.com") { t.Fatal("unexpected admin") }
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/auth -run TestIsAdmin_MatchesUsernameOrEmail -v`  
Expected: FAIL (function missing).

**Step 3: Write minimal implementation**

- Implement:
  - `func IsAdmin(cfg *models.Config, username, email string) bool`
  - `func RequireAdmin(cfg *models.Config) gin.HandlerFunc`
- Middleware behavior:
  - call `RequireAuth(c)` first
  - if not admin: return `403`.
  - set `c.Set("isAdmin", true)` on success.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/auth -run TestIsAdmin_MatchesUsernameOrEmail -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/auth/admin.go backend/internal/auth/admin_test.go
git commit -m "feat(auth): add admin allowlist middleware"
```

### Task 3: API Key Store Model And K8s Secret Backend

**Files:**
- Create: `backend/internal/models/apikey.go`
- Create: `backend/internal/k8s/apikeys.go`
- Test: `backend/internal/k8s/apikeys_test.go`

**Step 1: Write the failing test**

```go
func TestAPIKeyStore_CRUD(t *testing.T) {
    // fake clientset + empty secret
    // create record, list, update, delete
    // assert serialized JSON and deterministic ordering by CreatedAt desc
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/k8s -run TestAPIKeyStore_CRUD -v`  
Expected: FAIL (store not implemented).

**Step 3: Write minimal implementation**

- Secret constants:
  - name: `genet-openapi-keys` (config override optional)
  - key: `records.json`
- Methods on `k8s.Client`:
  - `ListOpenAPIKeys(ctx)`
  - `CreateOpenAPIKey(ctx, rec)`
  - `UpdateOpenAPIKey(ctx, rec)`
  - `DeleteOpenAPIKey(ctx, id)`
  - `FindOpenAPIKeyByPlaintext(ctx, token)` (hash compare)

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/k8s -run TestAPIKeyStore_CRUD -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/models/apikey.go backend/internal/k8s/apikeys.go backend/internal/k8s/apikeys_test.go
git commit -m "feat(openapi): add secret-backed apikey store"
```

### Task 4: Admin Handler And Routes

**Files:**
- Create: `backend/internal/handlers/admin.go`
- Modify: `backend/cmd/api/main.go`
- Test: `backend/internal/handlers/admin_test.go`

**Step 1: Write the failing test**

```go
func TestAdminMe_ForbiddenForNonAdmin(t *testing.T) {
    // gin router with AuthMiddleware + RequireAdmin
    // request from non-admin user
    // expect 403
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/handlers -run TestAdminMe_ForbiddenForNonAdmin -v`  
Expected: FAIL.

**Step 3: Write minimal implementation**

- `AdminHandler` endpoints:
  - `GetMe`
  - `ListAPIKeys`
  - `CreateAPIKey` (generate random key, hash, persist, return plaintext once)
  - `UpdateAPIKey`
  - `DeleteAPIKey`
- Wire routes in `main.go`:
  - `/api/admin/*` guarded by `auth.AuthMiddleware(config)` + `auth.RequireAdmin(config)`.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/handlers -run TestAdminMe_ForbiddenForNonAdmin -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/handlers/admin.go backend/internal/handlers/admin_test.go backend/cmd/api/main.go
git commit -m "feat(admin): add admin apikey management endpoints"
```

### Task 5: Upgrade OpenAPI API Key Middleware To Resolve Owner User

**Files:**
- Modify: `backend/internal/auth/apikey.go`
- Test: `backend/internal/auth/apikey_test.go` (new)

**Step 1: Write the failing test**

```go
func TestAPIKeyAuthMiddleware_SetsOwnerUserFromManagedKey(t *testing.T) {
    // request with valid managed key
    // expect context fields: openapiOwnerUser, openapiScope, authMethod=apikey
}
```

**Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/auth -run TestAPIKeyAuthMiddleware_SetsOwnerUserFromManagedKey -v`  
Expected: FAIL.

**Step 3: Write minimal implementation**

- Middleware lookup order:
  1. managed keys from Secret (hash compare)
  2. fallback to legacy `config.OpenAPI.APIKeys` (owner empty)
- On success set context:
  - `openapiKeyID`
  - `openapiOwnerUser`
  - `openapiScope`
- Reject disabled/expired keys.

**Step 4: Run test to verify it passes**

Run: `cd backend && go test ./internal/auth -run TestAPIKeyAuthMiddleware_SetsOwnerUserFromManagedKey -v`  
Expected: PASS.

**Step 5: Commit**

```bash
git add backend/internal/auth/apikey.go backend/internal/auth/apikey_test.go
git commit -m "feat(openapi): resolve owner user and scope from managed apikey"
```

### Task 6: Frontend API Client And Route Guard

**Files:**
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/App.tsx`

**Step 1: Write the failing test (manual contract check)**

- Add temporary compile references for new types/functions in page (next task).
- Build should fail before API additions.

**Step 2: Run check to verify it fails**

Run: `cd frontend && npm run build`  
Expected: FAIL (`getAdminMe` / admin API types missing).

**Step 3: Write minimal implementation**

- Add client methods:
  - `getAdminMe`
  - `listAdminAPIKeys`
  - `createAdminAPIKey`
  - `updateAdminAPIKey`
  - `deleteAdminAPIKey`
- Add `/admin/apikeys` route in `App.tsx`.

**Step 4: Run check to verify it passes**

Run: `cd frontend && npm run build`  
Expected: PASS (or move to next task if page not added yet).

**Step 5: Commit**

```bash
git add frontend/src/services/api.ts frontend/src/App.tsx
git commit -m "feat(frontend): add admin apikey api client and route wiring"
```

### Task 7: Admin Page UI (Create/Bind/Enable/Disable/Delete API Keys)

**Files:**
- Create: `frontend/src/pages/AdminAPIKeys/index.tsx`
- Create: `frontend/src/pages/AdminAPIKeys/index.css`
- Modify: `frontend/src/pages/Dashboard/index.tsx`

**Step 1: Write the failing test (manual)**

- Navigate to `/admin/apikeys`:
  - non-admin should see forbidden/redirect
  - admin should load table and actions

**Step 2: Run check to verify it fails**

Run: `cd frontend && npm run build`  
Expected: FAIL until page/export/import are complete.

**Step 3: Write minimal implementation**

- UI elements:
  - top summary (`current user`, `admin` badge)
  - table (`name`, `ownerUser`, `scope`, `enabled`, `updatedAt`)
  - create modal (name + ownerUser + scope + expiresAt)
  - edit modal (ownerUser/scope/enabled/expiresAt)
  - revoke action with confirmation
  - one-time plaintext key reveal modal
- Dashboard add entry button (visible only if `getAdminMe().isAdmin`).

**Step 4: Run check to verify it passes**

Run: `cd frontend && npm run build`  
Expected: PASS.

**Step 5: Commit**

```bash
git add frontend/src/pages/AdminAPIKeys/index.tsx frontend/src/pages/AdminAPIKeys/index.css frontend/src/pages/Dashboard/index.tsx
git commit -m "feat(frontend): add admin apikey management page"
```

### Task 8: Integration Verification And Docs

**Files:**
- Modify: `README.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `backend/api/openapi.yaml` (description updates for owner/scope behavior)

**Step 1: Write verification checklist**

1. OAuth user in `adminUsers` can open `/admin/apikeys`.
2. Non-admin gets `403` on `/api/admin/me` and admin APIs.
3. New API key returns plaintext once and later only preview.
4. OpenAPI request with new key resolves owner user in middleware context/log.
5. Disabled/expired key gets `401`.

**Step 2: Run backend tests**

Run: `cd backend && go test ./...`  
Expected: PASS.

**Step 3: Run frontend build**

Run: `cd frontend && npm run build`  
Expected: PASS.

**Step 4: Update docs**

- Helm sample:
  - `backend.config.adminUsers: ["alice", "ops@example.com"]`
- Admin page usage and key lifecycle.
- Explain legacy `openAPI.apiKeys` migration notes.

**Step 5: Commit**

```bash
git add README.md docs/ARCHITECTURE.md backend/api/openapi.yaml
git commit -m "docs: add admin apikey management and configuration guide"
```

---

## Rollout And Migration Notes

1. Deploy with `adminUsers` first, keep existing `openAPI.apiKeys`.
2. Create managed keys in admin page and migrate clients gradually.
3. Remove legacy static keys after traffic migration completes.

## Done Criteria

1. Admin-only page is accessible only for configured admins.
2. Admin can create/update/revoke keys and bind owner user.
3. OpenAPI middleware can resolve owner user from API key and reject invalid/expired keys.
4. Helm values fully control admin allowlist and OpenAPI settings.
5. Backend tests and frontend build pass.
