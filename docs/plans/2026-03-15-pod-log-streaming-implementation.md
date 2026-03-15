# Pod Log Streaming Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace current pod log polling with WebSocket streaming for current logs while keeping previous logs on the existing request path.

**Architecture:** The pod detail page will fetch a bounded slice of current logs once, then append new logs from a dedicated WebSocket endpoint. The backend will add a streaming endpoint plus cursor-aware HTTP log fetches to support initial history and reconnect backfill.

**Tech Stack:** React 18, TypeScript, Ant Design, Gin, gorilla/websocket, client-go, Jest, Go test

---

### Task 1: Backend HTTP log options

**Files:**
- Modify: `backend/internal/handlers/pod.go`
- Modify: `backend/internal/k8s/pod.go`
- Modify: `backend/internal/handlers/pod_logs_test.go`

**Step 1: Write the failing test**

Add handler tests that verify:
- `GET /pods/:id/logs?tailLines=200` forwards the requested tail size
- `GET /pods/:id/logs?since=<timestamp>` forwards the cursor and returns a cursor in the response

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers -run TestGetPodLogs`
Expected: FAIL because the handler only supports fixed tail lines and no cursor.

**Step 3: Write minimal implementation**

Add log query parsing and response metadata support in the handler and k8s client.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers -run TestGetPodLogs`
Expected: PASS

### Task 2: Backend WebSocket log stream

**Files:**
- Modify: `backend/cmd/api/main.go`
- Modify: `backend/internal/handlers/pod.go`
- Add: `backend/internal/handlers/pod_log_stream_test.go`

**Step 1: Write the failing test**

Add a WebSocket handler test that verifies `/pods/:id/logs/stream` upgrades and emits chunk messages from the pod log follow stream.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/handlers -run TestPodLogStream`
Expected: FAIL because the endpoint does not exist yet.

**Step 3: Write minimal implementation**

Add a WebSocket endpoint and line-oriented pod log stream loop that emits JSON chunk messages.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/handlers -run TestPodLogStream`
Expected: PASS

### Task 3: Frontend current log streaming

**Files:**
- Modify: `frontend/src/services/api.ts`
- Modify: `frontend/src/pages/PodDetail/index.tsx`
- Modify: `frontend/src/pages/PodDetail/index.css`
- Modify: `frontend/src/pages/PodDetail/index.test.tsx`

**Step 1: Write the failing test**

Add tests that verify:
- current logs load recent history and open one WebSocket
- incoming stream chunks append instead of replacing
- switching to previous logs avoids streaming
- log buffer trimming keeps the visible log size bounded

**Step 2: Run test to verify it fails**

Run: `npm test -- --runInBand --watch=false src/pages/PodDetail/index.test.tsx`
Expected: FAIL because the page still uses polling and replacement updates.

**Step 3: Write minimal implementation**

Wire current logs to initial HTTP fetch plus WebSocket append flow, keeping previous logs on manual fetches.

**Step 4: Run test to verify it passes**

Run: `npm test -- --runInBand --watch=false src/pages/PodDetail/index.test.tsx`
Expected: PASS

### Task 4: Verification

**Files:**
- No code changes expected

**Step 1: Run focused tests**

Run: `npm test -- --runInBand --watch=false src/pages/PodDetail/index.test.tsx`
Run: `go test ./internal/handlers -run 'TestGetPodLogs|TestPodLogStream'`

**Step 2: Run broader checks**

Run: `npm run build`
Run: `go test ./internal/handlers ./internal/k8s`

**Step 3: Report verified status only**

Summarize exact passing commands and any remaining gaps.
