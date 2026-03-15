# Workload Suspend Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add scheduled suspend behavior for `Deployment` and `StatefulSet` while preserving existing daily Pod cleanup, and allow suspended workloads to be resumed from the committed image.

**Architecture:** Extend the cleanup job to branch on workload type. Plain Pods keep the current delete flow. Managed workloads select one representative Pod, commit it to an image, persist suspend metadata in workload annotations, and scale replicas to zero only after commit succeeds. Add workload resume endpoints and expose suspended state in the dashboard cards.

**Tech Stack:** Go, Gin, client-go, React, TypeScript, Ant Design, Jest, React Testing Library

---

### Task 1: Document the suspend annotations in backend tests

**Files:**
- Modify: `backend/internal/handlers/deployment_test.go`
- Modify: `backend/internal/handlers/statefulset_test.go`

**Step 1: Write the failing test**

Add response tests asserting that a suspended workload is returned with:

- `status = Suspended`
- `suspended = true`
- `suspendedImage`
- `suspendedReplicas`

**Step 2: Run test to verify it fails**

Run:

- `go test ./internal/handlers -run TestBuildDeploymentResponse`
- `go test ./internal/handlers -run TestBuildStatefulSetResponse`

Expected: FAIL because suspend fields and `Suspended` mapping do not exist.

**Step 3: Write minimal implementation**

Extend workload response models and response builders to parse suspend annotations and map suspended workloads to `Suspended`.

**Step 4: Run test to verify it passes**

Run:

- `go test ./internal/handlers -run TestBuildDeploymentResponse`
- `go test ./internal/handlers -run TestBuildStatefulSetResponse`

Expected: PASS

### Task 2: Add failing cleanup tests for workload suspend flow

**Files:**
- Create: `backend/internal/cleanup/lifecycle_test.go`
- Modify: `backend/internal/cleanup/lifecycle.go`

**Step 1: Write the failing test**

Add tests covering:

- unmanaged single Pod cleanup still deletes Pod
- deployment suspend commits one representative Pod and scales to zero
- statefulset suspend commits one representative Pod and scales to zero
- commit failure leaves workload replicas unchanged

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cleanup -run TestCleanup`
Expected: FAIL because cleanup only deletes Pods today.

**Step 3: Write minimal implementation**

Refactor cleanup into separable helpers for:

- plain Pod cleanup
- representative Pod selection
- workload suspend

Inject helper functions where needed so tests can stub commit behavior without a live cluster.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cleanup -run TestCleanup`
Expected: PASS

### Task 3: Add k8s workload suspend/resume helpers

**Files:**
- Create: `backend/internal/k8s/workload_suspend.go`
- Create: `backend/internal/k8s/workload_suspend_test.go`
- Modify: `backend/internal/k8s/deployment.go`
- Modify: `backend/internal/k8s/statefulset.go`

**Step 1: Write the failing test**

Add tests covering:

- selecting a representative Pod prefers the first running Pod by name
- suspending a deployment updates annotations, image, and replicas
- resuming a deployment restores replicas and keeps the committed image
- suspending and resuming a statefulset behaves the same way

**Step 2: Run test to verify it fails**

Run: `go test ./internal/k8s -run 'Test(SelectRepresentativePod|Suspend|Resume)'`
Expected: FAIL because helper methods do not exist.

**Step 3: Write minimal implementation**

Implement reusable helpers for:

- representative Pod selection
- suspend annotation parse/write
- deployment/statefulset image + replica updates

**Step 4: Run test to verify it passes**

Run: `go test ./internal/k8s -run 'Test(SelectRepresentativePod|Suspend|Resume)'`
Expected: PASS

### Task 4: Add resume handler tests and routes

**Files:**
- Modify: `backend/internal/handlers/deployment.go`
- Modify: `backend/internal/handlers/deployment_test.go`
- Modify: `backend/internal/handlers/statefulset.go`
- Modify: `backend/internal/handlers/statefulset_test.go`
- Modify: `backend/cmd/api/main.go`

**Step 1: Write the failing test**

Add handler tests covering:

- `POST /deployments/:id/resume` restores replicas from suspend annotations
- `POST /statefulsets/:id/resume` restores replicas from suspend annotations
- non-suspended workloads return conflict

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers -run 'Test(ResumeDeployment|ResumeStatefulSet)'`
Expected: FAIL because resume endpoints do not exist.

**Step 3: Write minimal implementation**

Add resume handlers and routes, then delegate to the new k8s suspend/resume helpers.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers -run 'Test(ResumeDeployment|ResumeStatefulSet)'`
Expected: PASS

### Task 5: Add frontend failing tests for suspended cards

**Files:**
- Modify: `frontend/src/pages/Dashboard/Dashboard.test.tsx`
- Create or Modify: `frontend/src/pages/Dashboard/DeploymentCard.test.tsx`
- Create or Modify: `frontend/src/pages/Dashboard/StatefulSetCard.test.tsx`
- Modify: `frontend/src/services/api.ts`

**Step 1: Write the failing test**

Add tests covering:

- suspended deployment card shows `挂起`
- suspended statefulset card shows `挂起`
- clicking `恢复` calls the correct API and refresh callback

**Step 2: Run test to verify it fails**

Run: `npm test -- --watch=false --runInBand src/pages/Dashboard/DeploymentCard.test.tsx src/pages/Dashboard/StatefulSetCard.test.tsx`
Expected: FAIL because suspend fields and resume actions are missing.

**Step 3: Write minimal implementation**

Extend frontend types and API helpers, then update workload cards to render suspended state and recovery actions.

**Step 4: Run test to verify it passes**

Run: `npm test -- --watch=false --runInBand src/pages/Dashboard/DeploymentCard.test.tsx src/pages/Dashboard/StatefulSetCard.test.tsx`
Expected: PASS

### Task 6: Run focused verification

**Files:**
- Modify: `backend/internal/cleanup/lifecycle.go`
- Modify: `backend/internal/handlers/deployment.go`
- Modify: `backend/internal/handlers/statefulset.go`
- Modify: `backend/internal/k8s/deployment.go`
- Modify: `backend/internal/k8s/statefulset.go`
- Modify: `frontend/src/pages/Dashboard/DeploymentCard.tsx`
- Modify: `frontend/src/pages/Dashboard/StatefulSetCard.tsx`
- Modify: `frontend/src/services/api.ts`

**Step 1: Run Go tests**

Run:

- `go test ./internal/cleanup ./internal/k8s ./internal/handlers`

Expected: PASS

**Step 2: Run frontend tests**

Run:

- `npm test -- --watch=false --runInBand src/pages/Dashboard/DeploymentCard.test.tsx src/pages/Dashboard/StatefulSetCard.test.tsx src/pages/Dashboard/Dashboard.test.tsx`

Expected: PASS

**Step 3: Run formatting if needed**

Run:

- `gofmt -w backend/internal/cleanup/lifecycle.go backend/internal/k8s/workload_suspend.go backend/internal/handlers/deployment.go backend/internal/handlers/statefulset.go`

Expected: files remain properly formatted
