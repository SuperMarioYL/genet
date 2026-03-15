# Pod Log Follow And Previous Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a follow-latest mode to the pod detail live logs view and let users switch to previous pod logs.

**Architecture:** Extend the pod detail logs tab with a dedicated log source toggle plus follow-state-aware scrolling. Extend the pod logs API to accept a `previous` query flag and propagate it through the pod handler into Kubernetes log options.

**Tech Stack:** React 18, Ant Design, TypeScript, Gin, client-go, Jest, Go test

---

### Task 1: Frontend log source and follow behavior

**Files:**
- Modify: `frontend/src/pages/PodDetail/index.tsx`
- Modify: `frontend/src/pages/PodDetail/index.css`
- Modify: `frontend/src/pages/PodDetail/index.test.tsx`
- Modify: `frontend/src/services/api.ts`

**Step 1: Write the failing test**

Add tests that verify:
- refreshing logs in “之前日志” mode calls `getPodLogs(id, { previous: true })`
- follow mode scrolls the log container to the bottom after logs update
- manual upward scrolling disables follow mode

**Step 2: Run test to verify it fails**

Run: `npm test -- --runInBand --watch=false src/pages/PodDetail/index.test.tsx`
Expected: FAIL because the UI and API helper do not yet support previous logs or follow-state scrolling.

**Step 3: Write minimal implementation**

Implement:
- a current/previous log source toggle in the logs toolbar
- a follow switch defaulting to enabled
- a scroll handler that disables follow when the user leaves the bottom
- auto-scroll to bottom after log updates while follow mode is active
- API helper support for `previous`

**Step 4: Run test to verify it passes**

Run: `npm test -- --runInBand --watch=false src/pages/PodDetail/index.test.tsx`
Expected: PASS

### Task 2: Backend previous-log plumbing

**Files:**
- Modify: `backend/internal/handlers/pod.go`
- Add: `backend/internal/handlers/pod_logs_test.go`
- Modify: `backend/internal/k8s/pod.go`

**Step 1: Write the failing test**

Add a handler test that verifies `GET /pods/:id/logs?previous=true` calls the pod log reader with `previous=true` and returns the resulting logs.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers -run TestGetPodLogsSupportsPreviousLogs`
Expected: FAIL because the handler does not parse or forward the `previous` flag.

**Step 3: Write minimal implementation**

Implement:
- query parsing for `previous`
- a pod-handler log reader hook for testability
- K8s client support for `PodLogOptions.Previous`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers -run TestGetPodLogsSupportsPreviousLogs`
Expected: PASS

### Task 3: Final verification

**Files:**
- No code changes expected

**Step 1: Run targeted frontend and backend tests**

Run: `npm test -- --runInBand --watch=false src/pages/PodDetail/index.test.tsx`
Run: `go test ./internal/handlers -run TestGetPodLogsSupportsPreviousLogs`

**Step 2: Run broader safety checks**

Run: `npm run build`
Run: `go test ./internal/handlers ./internal/k8s`

**Step 3: Inspect results and report only verified status**

Summarize passing/failing commands and any remaining gaps before claiming completion.
