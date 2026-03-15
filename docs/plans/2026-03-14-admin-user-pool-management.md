# Admin User Pool Management Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a unified user/admin entry, an admin console for node pool and user pool drag-and-drop management, and enforce user pool bindings during Pod scheduling.

**Architecture:** Add admin APIs for node pool and user pool management on top of existing admin auth, persist user pool bindings in Kubernetes, and apply the resolved pool as scheduling constraints in the Pod creation path. On the frontend, add a shared user menu, a personal details page, and a unified admin page with tabs for overview, node pool management, user pool management, and API key management. Limit pool management lists to compute-related entities and allow admins to force-delete a user together with namespace resources.

**Tech Stack:** Go, Gin, client-go, React, TypeScript, Ant Design, Jest, React Testing Library

---

### Task 1: Add backend models for admin pool management

**Files:**
- Modify: `backend/internal/handlers/admin.go`
- Modify: `frontend/src/services/api.ts`
- Test: `backend/internal/handlers/admin_test.go`

**Step 1: Write the failing test**

Add handler response shape tests for user pool and node pool payloads in `backend/internal/handlers/admin_test.go`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers -run 'TestAdmin.*Pool'`
Expected: FAIL because the new types and endpoints do not exist.

**Step 3: Write minimal implementation**

Add request and response structs for:

- admin overview
- admin node pool list and update
- admin user pool list and update

Add matching frontend TypeScript interfaces and API helpers in `frontend/src/services/api.ts`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers -run 'TestAdmin.*Pool'`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/handlers/admin.go backend/internal/handlers/admin_test.go frontend/src/services/api.ts
git commit -m "feat: add admin pool api models"
```

### Task 2: Persist user pool bindings in Kubernetes

**Files:**
- Modify: `backend/internal/k8s/client.go`
- Create: `backend/internal/k8s/user_pool_store.go`
- Create: `backend/internal/k8s/user_pool_store_test.go`

**Step 1: Write the failing test**

Add tests covering:

- reading empty bindings defaults to no records
- updating a user binding writes `shared` or `exclusive`
- updating an existing user overwrites prior pool

**Step 2: Run test to verify it fails**

Run: `go test ./internal/k8s -run 'TestUserPool'`
Expected: FAIL because the store does not exist.

**Step 3: Write minimal implementation**

Implement a small store backed by a Kubernetes `ConfigMap` named `genet-user-pool-bindings` with helpers to:

- list bindings
- get binding for a username
- upsert binding

Default missing users to `shared` at the call site instead of materializing defaults in storage.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/k8s -run 'TestUserPool'`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/k8s/client.go backend/internal/k8s/user_pool_store.go backend/internal/k8s/user_pool_store_test.go
git commit -m "feat: persist admin user pool bindings"
```

### Task 3: Add admin endpoints for overview, node pools, and user pools

**Files:**
- Modify: `backend/internal/handlers/admin.go`
- Modify: `backend/cmd/api/main.go`
- Test: `backend/internal/handlers/admin_test.go`

**Step 1: Write the failing test**

Add handler tests for:

- listing node pools
- updating one node from `shared` to `exclusive`
- listing users with resolved pool assignments
- updating one user pool assignment

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers -run 'TestAdmin(List|Update).*(Node|User)Pool'`
Expected: FAIL because routes and methods are missing.

**Step 3: Write minimal implementation**

Implement:

- `GET /api/admin/overview`
- `GET /api/admin/nodes/pools`
- `PATCH /api/admin/nodes/:name/pool`
- `GET /api/admin/users/pools`
- `PATCH /api/admin/users/:username/pool`

Node pool updates should mutate the existing `genet.io/node-pool` label. User lists should merge:

- active Genet Pods
- Genet Deployments and StatefulSets
- stored bindings

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers -run 'TestAdmin(List|Update).*(Node|User)Pool'`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/cmd/api/main.go backend/internal/handlers/admin.go backend/internal/handlers/admin_test.go
git commit -m "feat: add admin node and user pool endpoints"
```

### Task 3.1: Filter management lists and support force-deleting users

**Files:**
- Modify: `backend/internal/handlers/admin.go`
- Modify: `backend/internal/k8s/namespace.go`
- Modify: `backend/internal/k8s/user_pool_store.go`
- Modify: `backend/cmd/api/main.go`
- Modify: `frontend/src/pages/Admin/index.tsx`
- Modify: `frontend/src/pages/Admin/index.css`
- Modify: `frontend/src/services/api.ts`
- Test: `backend/internal/handlers/admin_test.go`
- Test: `frontend/src/pages/Admin/index.test.tsx`

**Step 1: Write the failing test**

Add tests covering:

- namespace-only users are excluded from user management
- `DELETE /api/admin/users/:username` removes the user pool binding and deletes user resources
- the admin page exposes a delete action for each user card

**Step 2: Run test to verify it fails**

Run:

- `go test ./internal/handlers -run 'TestAdmin(ListUserPools_OK|DeleteUser_OK)$'`
- `npm test -- --watch=false --runInBand src/pages/Admin/index.test.tsx`

Expected: FAIL because the filter and delete flow do not exist yet.

**Step 3: Write minimal implementation**

Implement:

- user list aggregation from bindings + actual workloads only
- `DELETE /api/admin/users/:username`
- namespace force-delete helper for the user namespace and its main resources
- frontend delete button and optimistic refresh

**Step 4: Run test to verify it passes**

Run:

- `go test ./internal/handlers -run 'TestAdmin(ListUserPools_OK|DeleteUser_OK)$'`
- `npm test -- --watch=false --runInBand src/pages/Admin/index.test.tsx`

Expected: PASS

### Task 4: Enforce user pool bindings in Pod scheduling

**Files:**
- Modify: `backend/internal/handlers/pod.go`
- Modify: `backend/internal/k8s/pod.go`
- Create: `backend/internal/handlers/pod_user_pool_test.go`
- Create: `backend/internal/k8s/pod_user_pool_test.go`

**Step 1: Write the failing test**

Add tests covering:

- user bound to `exclusive` gets exclusive-only scheduling constraints
- user bound to `shared` gets shared-only scheduling constraints
- explicitly requested node outside the user pool is rejected
- exclusive pool Pod tolerates the non-shared taint

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers ./internal/k8s -run 'Test(CreatePod|BuildPod).*UserPool'`
Expected: FAIL because the scheduling constraints are not applied.

**Step 3: Write minimal implementation**

Resolve current user pool before Pod creation, validate the requested node if present, and inject node affinity or tolerations into the generated Pod spec. Reuse existing node pool label and taint constants instead of inventing parallel identifiers.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers ./internal/k8s -run 'Test(CreatePod|BuildPod).*UserPool'`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/handlers/pod.go backend/internal/handlers/pod_user_pool_test.go backend/internal/k8s/pod.go backend/internal/k8s/pod_user_pool_test.go
git commit -m "feat: enforce user pool scheduling"
```

### Task 5: Add the shared user menu and personal details page

**Files:**
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/pages/Dashboard/index.tsx`
- Create: `frontend/src/components/UserMenu.tsx`
- Create: `frontend/src/components/UserMenu.css`
- Create: `frontend/src/pages/Profile/index.tsx`
- Create: `frontend/src/pages/Profile/index.css`
- Test: `frontend/src/pages/Dashboard/Dashboard.test.tsx`
- Create: `frontend/src/pages/Profile/index.test.tsx`

**Step 1: Write the failing test**

Add tests proving:

- the dashboard header shows a user menu entry
- non-admin users are routed to the personal details page
- the profile page renders current pool information

**Step 2: Run test to verify it fails**

Run: `npm test -- --runInBand Dashboard.test.tsx Profile/index.test.tsx`
Expected: FAIL because the user menu and profile page do not exist.

**Step 3: Write minimal implementation**

Create a reusable header user menu component driven by `getAdminMe`. Add `/me` route and the profile page. Keep the existing header look and move admin-only entry points into the menu.

**Step 4: Run test to verify it passes**

Run: `npm test -- --runInBand Dashboard.test.tsx Profile/index.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/pages/Dashboard/index.tsx frontend/src/components/UserMenu.tsx frontend/src/components/UserMenu.css frontend/src/pages/Profile/index.tsx frontend/src/pages/Profile/index.css frontend/src/pages/Dashboard/Dashboard.test.tsx frontend/src/pages/Profile/index.test.tsx
git commit -m "feat: add user menu and profile page"
```

### Task 6: Build the unified admin page

**Files:**
- Modify: `frontend/src/App.tsx`
- Create: `frontend/src/pages/Admin/index.tsx`
- Create: `frontend/src/pages/Admin/index.css`
- Modify: `frontend/src/pages/AdminAPIKeys/index.tsx`
- Test: `frontend/src/pages/AdminAPIKeys/index.tsx`
- Create: `frontend/src/pages/Admin/index.test.tsx`

**Step 1: Write the failing test**

Add tests proving:

- admin users can open the admin page
- the page renders the four tabs
- node pool and user pool lists load from the new APIs

**Step 2: Run test to verify it fails**

Run: `npm test -- --runInBand Admin/index.test.tsx`
Expected: FAIL because the admin page does not exist.

**Step 3: Write minimal implementation**

Create a unified admin page with tabs:

- overview
- node pool management
- user pool management
- API key management

Refactor the existing API key page into an embeddable panel component or reuse its content inside the new admin page.

**Step 4: Run test to verify it passes**

Run: `npm test -- --runInBand Admin/index.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/App.tsx frontend/src/pages/Admin/index.tsx frontend/src/pages/Admin/index.css frontend/src/pages/AdminAPIKeys/index.tsx frontend/src/pages/Admin/index.test.tsx
git commit -m "feat: add unified admin console"
```

### Task 7: Add drag-and-drop pool operations on the admin page

**Files:**
- Modify: `frontend/src/pages/Admin/index.tsx`
- Modify: `frontend/src/pages/Admin/index.css`
- Test: `frontend/src/pages/Admin/index.test.tsx`

**Step 1: Write the failing test**

Add tests proving:

- dragging a node card to the other pool calls the node update API
- dragging a user card to the other pool calls the user update API
- failed updates restore the prior visual grouping

**Step 2: Run test to verify it fails**

Run: `npm test -- --runInBand Admin/index.test.tsx`
Expected: FAIL because drag-and-drop behavior is not implemented.

**Step 3: Write minimal implementation**

Implement native HTML5 drag and drop between the two columns for nodes and users. Use optimistic UI only if rollback logic is explicit and covered by tests.

**Step 4: Run test to verify it passes**

Run: `npm test -- --runInBand Admin/index.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/pages/Admin/index.tsx frontend/src/pages/Admin/index.css frontend/src/pages/Admin/index.test.tsx
git commit -m "feat: add admin pool drag and drop"
```

### Task 8: Run full verification and document any residual gaps

**Files:**
- Modify: `docs/USER_GUIDE.md`

**Step 1: Verify backend tests**

Run: `go test ./...`
Expected: PASS

**Step 2: Verify frontend tests**

Run: `npm test -- --runInBand`
Expected: PASS

**Step 3: Verify frontend build**

Run: `npm run build`
Expected: PASS

**Step 4: Update docs**

Document:

- personal page entry
- admin page tabs
- node pool management
- user pool binding behavior

**Step 5: Commit**

```bash
git add docs/USER_GUIDE.md
git commit -m "docs: describe admin pool management"
```
