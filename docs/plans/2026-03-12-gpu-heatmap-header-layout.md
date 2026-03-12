# GPU Heatmap Header Layout Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show the GPU heatmap title and summary on one row in the modal header, with the summary metrics rendered as a single line.

**Architecture:** Keep the modal header as the only visible title source. `AcceleratorHeatmap` remains responsible for fetching overview data, and reports the latest summary upward via a callback so the Dashboard can render the header summary without duplicating requests.

**Tech Stack:** React 18, TypeScript, Ant Design, Jest, react-scripts test

---

### Task 1: Lock the modal title behavior with a failing test

**Files:**
- Modify: `frontend/src/pages/Dashboard/Dashboard.test.tsx`

**Step 1: Write the failing test**

Add a dashboard test that opens the heatmap modal and checks that the title row contains:
- `GPU 热力图`
- `总卡数量`
- `占用量`

**Step 2: Run test to verify it fails**

Run: `CI=1 npm test -- --runInBand --watch=false Dashboard.test.tsx`

Expected: FAIL because the modal title does not yet render the summary line.

### Task 2: Surface summary data into the modal title

**Files:**
- Modify: `frontend/src/components/AcceleratorHeatmap/index.tsx`
- Modify: `frontend/src/components/AcceleratorHeatmap/AcceleratorHeatmap.css`
- Modify: `frontend/src/pages/Dashboard/index.tsx`
- Modify: `frontend/src/pages/Dashboard/index.css`

**Step 1: Add minimal data callback**

Extend `AcceleratorHeatmap` with an optional summary callback prop and invoke it whenever overview data loads successfully.

**Step 2: Render the modal title summary**

Store the latest summary in Dashboard state and render a compact title row:
- left: icon + `GPU 热力图`
- right: `总卡数量` and `占用量`

**Step 3: Simplify the inner heatmap header**

Remove the duplicated summary from the heatmap body header and keep only the refresh control there.

### Task 3: Verify the change

**Files:**
- Test: `frontend/src/pages/Dashboard/Dashboard.test.tsx`
- Test: `frontend/src/components/AcceleratorHeatmap/AcceleratorHeatmap.test.tsx`

**Step 1: Run targeted tests**

Run: `CI=1 npm test -- --runInBand --watch=false Dashboard.test.tsx AcceleratorHeatmap.test.tsx`

**Step 2: Confirm output**

Expected: PASS with the modal title summary visible and no duplicate internal title text.
