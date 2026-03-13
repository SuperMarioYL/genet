# Service ConfigMap Job OpenAPI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add simplified OpenAPI CRUD for `Service` and `ConfigMap`, and migrate `Job` OpenAPI from Kubernetes-native YAML to the Genet simplified JSON model while keeping ownership and runtime behavior consistent with existing Pod APIs.

**Architecture:** Extend `/api/open/*` with resource-specific simplified DTOs, split OpenAPI handlers by resource, and introduce conversion/build layers that turn simplified requests into Kubernetes-native resources. Extract shared Pod runtime construction so Pod and Job use the same resource, GPU, mount, proxy, and scheduling logic.

**Tech Stack:** Go, Gin, Kubernetes client-go, OpenAPI YAML, existing Genet auth and k8s client layers, Go test tooling.

---

### Task 1: Add OpenAPI DTOs for Service ConfigMap and Job

**Files:**
- Create: `backend/internal/models/openapi_service.go`
- Create: `backend/internal/models/openapi_configmap.go`
- Create: `backend/internal/models/openapi_job.go`
- Test: `backend/internal/models/openapi_models_test.go`

**Step 1: Write the failing test**

```go
func TestOpenAPIServiceRequestValidationTagsExist(t *testing.T) {
    req := OpenAPIServiceRequest{}
    if req.Name != "" {
        t.Fatal("expected zero value request")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./backend/internal/models -run TestOpenAPIServiceRequestValidationTagsExist -v`
Expected: FAIL with build errors because the OpenAPI request types do not exist.

**Step 3: Write minimal implementation**

Create request/response DTOs for:

- `OpenAPIServiceRequest`
- `OpenAPIServicePort`
- `OpenAPIServiceResponse`
- `OpenAPIServiceListResponse`
- `OpenAPIConfigMapRequest`
- `OpenAPIConfigMapSummary`
- `OpenAPIConfigMapDetail`
- `OpenAPIConfigMapListResponse`
- `OpenAPIJobRequest`
- `OpenAPIEnvVar`
- `OpenAPIJobResponse`
- `OpenAPIJobListResponse`

Include json tags and validation-oriented structure matching the approved design.

**Step 4: Run test to verify it passes**

Run: `go test ./backend/internal/models -run TestOpenAPIServiceRequestValidationTagsExist -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/models/openapi_service.go backend/internal/models/openapi_configmap.go backend/internal/models/openapi_job.go backend/internal/models/openapi_models_test.go
git commit -m "feat: add openapi resource models"
```

### Task 2: Add request validation helpers for simplified resources

**Files:**
- Modify: `backend/internal/handlers/validation.go`
- Test: `backend/internal/handlers/validation_test.go`

**Step 1: Write the failing test**

```go
func TestValidateOpenAPIServiceRequestRejectsMissingSelector(t *testing.T) {
    req := models.OpenAPIServiceRequest{
        Name:  "svc-demo",
        Type:  "ClusterIP",
        Ports: []models.OpenAPIServicePort{{Port: 80, TargetPort: "8080"}},
    }

    err := ValidateOpenAPIServiceRequest(&req)
    if err == nil {
        t.Fatal("expected selector validation error")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./backend/internal/handlers -run TestValidateOpenAPIServiceRequestRejectsMissingSelector -v`
Expected: FAIL because the validation helper does not exist.

**Step 3: Write minimal implementation**

Add validation helpers for:

- service name and selector rules
- service port shape and `nodePort` restrictions
- configmap data or binaryData presence and base64 validation
- reserved annotation prefix blocking
- job runtime and job control fields

Keep Pod validation untouched except for shared helper extraction if needed.

**Step 4: Run test to verify it passes**

Run: `go test ./backend/internal/handlers -run TestValidateOpenAPIServiceRequestRejectsMissingSelector -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/handlers/validation.go backend/internal/handlers/validation_test.go
git commit -m "feat: validate simplified openapi resources"
```

### Task 3: Extract shared workload runtime builder from Pod creation

**Files:**
- Create: `backend/internal/k8s/workload_runtime.go`
- Modify: `backend/internal/k8s/pod.go`
- Test: `backend/internal/k8s/workload_runtime_test.go`
- Test: `backend/internal/k8s/pod_test.go`

**Step 1: Write the failing test**

```go
func TestBuildWorkloadRuntimeIncludesDownwardEnvAndShm(t *testing.T) {
    spec := &WorkloadRuntimeSpec{
        Name:     "demo",
        Username: "alice",
        Image:    "busybox:latest",
        CPU:      "2",
        Memory:   "4Gi",
        ShmSize:  "1Gi",
    }

    runtimeSpec, err := buildWorkloadRuntime(spec)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if len(runtimeSpec.Container.Env) == 0 {
        t.Fatal("expected downward env vars")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./backend/internal/k8s -run TestBuildWorkloadRuntimeIncludesDownwardEnvAndShm -v`
Expected: FAIL because the shared runtime builder does not exist.

**Step 3: Write minimal implementation**

Extract from `CreatePod` the reusable runtime assembly for:

- container image, command, args, working dir
- resource requests and limits
- proxy env vars
- downward API env vars
- GPU / accelerator env and resources
- shm and storage mounts
- user mounts
- runtime class, node selector, affinity, dns, extra volumes

Return a struct usable by both Pod and Job builders.

**Step 4: Run test to verify it passes**

Run: `go test ./backend/internal/k8s -run TestBuildWorkloadRuntimeIncludesDownwardEnvAndShm -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/k8s/workload_runtime.go backend/internal/k8s/pod.go backend/internal/k8s/workload_runtime_test.go backend/internal/k8s/pod_test.go
git commit -m "refactor: extract shared workload runtime builder"
```

### Task 4: Add Service builder and k8s client CRUD helpers

**Files:**
- Create: `backend/internal/k8s/service_builder.go`
- Modify: `backend/internal/k8s/client.go`
- Test: `backend/internal/k8s/service_builder_test.go`

**Step 1: Write the failing test**

```go
func TestBuildServiceFromRequestUsesTargetPodSelector(t *testing.T) {
    req := &models.OpenAPIServiceRequest{
        Name:          "svc-demo",
        Type:          "ClusterIP",
        TargetPodName: "pod-alice-train",
        Ports: []models.OpenAPIServicePort{
            {Name: "http", Port: 80, TargetPort: "8080"},
        },
    }

    svc, err := BuildServiceFromOpenAPIRequest("user-alice", "alice", req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if svc.Spec.Selector["app"] != "pod-alice-train" {
        t.Fatal("expected selector from target pod")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./backend/internal/k8s -run TestBuildServiceFromRequestUsesTargetPodSelector -v`
Expected: FAIL because the builder does not exist.

**Step 3: Write minimal implementation**

Create builder and client helpers for:

- build service object from simplified request
- create service
- list services by selector
- get service
- update service
- delete service

Inject standard labels and annotations in the builder path.

**Step 4: Run test to verify it passes**

Run: `go test ./backend/internal/k8s -run TestBuildServiceFromRequestUsesTargetPodSelector -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/k8s/service_builder.go backend/internal/k8s/client.go backend/internal/k8s/service_builder_test.go
git commit -m "feat: add openapi service builders"
```

### Task 5: Add ConfigMap builder and k8s client CRUD helpers

**Files:**
- Create: `backend/internal/k8s/configmap_builder.go`
- Modify: `backend/internal/k8s/client.go`
- Test: `backend/internal/k8s/configmap_builder_test.go`

**Step 1: Write the failing test**

```go
func TestBuildConfigMapFromRequestDecodesBinaryData(t *testing.T) {
    req := &models.OpenAPIConfigMapRequest{
        Name:       "cm-demo",
        BinaryData: map[string]string{"app.bin": "aGVsbG8="},
    }

    cm, err := BuildConfigMapFromOpenAPIRequest("user-alice", "alice", req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if string(cm.BinaryData["app.bin"]) != "hello" {
        t.Fatal("expected decoded binaryData")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./backend/internal/k8s -run TestBuildConfigMapFromRequestDecodesBinaryData -v`
Expected: FAIL because the builder does not exist.

**Step 3: Write minimal implementation**

Create builder and client helpers for:

- build configmap from simplified request
- create configmap
- list configmaps by selector
- get configmap
- update configmap
- delete configmap

Reject updates to immutable configmaps at handler or client boundary.

**Step 4: Run test to verify it passes**

Run: `go test ./backend/internal/k8s -run TestBuildConfigMapFromRequestDecodesBinaryData -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/k8s/configmap_builder.go backend/internal/k8s/client.go backend/internal/k8s/configmap_builder_test.go
git commit -m "feat: add openapi configmap builders"
```

### Task 6: Add Job builder based on simplified request and shared runtime

**Files:**
- Create: `backend/internal/k8s/job_builder.go`
- Modify: `backend/internal/k8s/pod.go`
- Test: `backend/internal/k8s/job_builder_test.go`

**Step 1: Write the failing test**

```go
func TestBuildJobFromRequestUsesSharedRuntimeResources(t *testing.T) {
    req := &models.OpenAPIJobRequest{
        Name:    "job-demo",
        Image:   "busybox:latest",
        CPU:     "4",
        Memory:  "8Gi",
        Command: []string{"sh"},
        Args:    []string{"-c", "echo hi"},
    }

    job, err := BuildJobFromOpenAPIRequest("user-alice", "alice", req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    got := job.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().String()
    if got != "4" {
        t.Fatalf("expected cpu request 4, got %s", got)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./backend/internal/k8s -run TestBuildJobFromRequestUsesSharedRuntimeResources -v`
Expected: FAIL because the simplified job builder does not exist.

**Step 3: Write minimal implementation**

Build a simplified Job converter that:

- uses the shared workload runtime builder
- wraps the runtime into `Job.Spec.Template`
- applies job control fields
- injects OpenAPI ownership labels

Do not retain YAML passthrough behavior in the new builder.

**Step 4: Run test to verify it passes**

Run: `go test ./backend/internal/k8s -run TestBuildJobFromRequestUsesSharedRuntimeResources -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/k8s/job_builder.go backend/internal/k8s/pod.go backend/internal/k8s/job_builder_test.go
git commit -m "feat: add simplified openapi job builder"
```

### Task 7: Split OpenAPI handlers and implement Service endpoints

**Files:**
- Create: `backend/internal/handlers/openapi_common.go`
- Create: `backend/internal/handlers/openapi_service.go`
- Modify: `backend/internal/handlers/openapi.go`
- Modify: `backend/cmd/api/main.go`
- Test: `backend/internal/handlers/openapi_service_test.go`

**Step 1: Write the failing test**

```go
func TestOpenAPIServiceCreateUsesOwnerNamespace(t *testing.T) {
    // Build gin context with openapiOwnerUser set and post JSON request.
    // Assert created service namespace equals derived user namespace.
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./backend/internal/handlers -run TestOpenAPIServiceCreateUsesOwnerNamespace -v`
Expected: FAIL because the service endpoints do not exist.

**Step 3: Write minimal implementation**

Implement:

- create/list/get/update/delete service handlers
- owner label enforcement
- `targetPodName` existence checks
- list selector appending with `genet.io/open-api=true` and owner label
- route registration under `/api/open/services`

**Step 4: Run test to verify it passes**

Run: `go test ./backend/internal/handlers -run TestOpenAPIServiceCreateUsesOwnerNamespace -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/handlers/openapi_common.go backend/internal/handlers/openapi_service.go backend/internal/handlers/openapi.go backend/cmd/api/main.go backend/internal/handlers/openapi_service_test.go
git commit -m "feat: add openapi service handlers"
```

### Task 8: Implement ConfigMap endpoints

**Files:**
- Create: `backend/internal/handlers/openapi_configmap.go`
- Modify: `backend/internal/handlers/openapi.go`
- Modify: `backend/cmd/api/main.go`
- Test: `backend/internal/handlers/openapi_configmap_test.go`

**Step 1: Write the failing test**

```go
func TestOpenAPIConfigMapUpdateRejectsImmutableConfigMap(t *testing.T) {
    // Seed immutable configmap and assert PUT returns 409.
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./backend/internal/handlers -run TestOpenAPIConfigMapUpdateRejectsImmutableConfigMap -v`
Expected: FAIL because the configmap endpoints do not exist.

**Step 3: Write minimal implementation**

Implement:

- create/list/get/update/delete configmap handlers
- list summary responses and detail get responses
- immutable update conflict handling
- route registration under `/api/open/configmaps`

**Step 4: Run test to verify it passes**

Run: `go test ./backend/internal/handlers -run TestOpenAPIConfigMapUpdateRejectsImmutableConfigMap -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/handlers/openapi_configmap.go backend/internal/handlers/openapi.go backend/cmd/api/main.go backend/internal/handlers/openapi_configmap_test.go
git commit -m "feat: add openapi configmap handlers"
```

### Task 9: Migrate Job endpoints from YAML passthrough to simplified JSON

**Files:**
- Create: `backend/internal/handlers/openapi_job.go`
- Modify: `backend/internal/handlers/openapi.go`
- Modify: `backend/cmd/api/main.go`
- Test: `backend/internal/handlers/openapi_job_test.go`

**Step 1: Write the failing test**

```go
func TestOpenAPIJobCreateAcceptsSimplifiedJSON(t *testing.T) {
    // Post simplified JSON and assert created job image and namespace.
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./backend/internal/handlers -run TestOpenAPIJobCreateAcceptsSimplifiedJSON -v`
Expected: FAIL because jobs still expect YAML/native schema.

**Step 3: Write minimal implementation**

Implement:

- simplified create/list/get/update/delete job handlers
- active job update conflict detection
- recreate-on-update behavior
- route behavior unchanged, payload format changed to JSON

Remove YAML binding and native Job schema handling from the public OpenAPI path.

**Step 4: Run test to verify it passes**

Run: `go test ./backend/internal/handlers -run TestOpenAPIJobCreateAcceptsSimplifiedJSON -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend/internal/handlers/openapi_job.go backend/internal/handlers/openapi.go backend/cmd/api/main.go backend/internal/handlers/openapi_job_test.go
git commit -m "feat: simplify openapi job endpoints"
```

### Task 10: Update OpenAPI specification and examples

**Files:**
- Modify: `backend/api/openapi.yaml`
- Test: `backend/api/openapi.yaml`

**Step 1: Write the failing test**

Use schema review instead of code test for this documentation task.

Check:

- `/api/open/services` paths missing
- `/api/open/configmaps` paths missing
- `/api/open/jobs` still documents YAML input

**Step 2: Run verification to show current gaps**

Run: `rg -n "/api/open/services|/api/open/configmaps|application/yaml|Job YAML" backend/api/openapi.yaml`
Expected: only job YAML references exist and service/configmap paths are missing.

**Step 3: Write minimal implementation**

Update `openapi.yaml` to:

- add service CRUD path items
- add configmap CRUD path items
- convert job request/response docs to simplified JSON schemas
- add reusable components for ports, env vars, mounts, summary/detail responses
- document owner namespace and server-managed labels

**Step 4: Run verification to confirm**

Run: `rg -n "/api/open/services|/api/open/configmaps|application/yaml|OpenAPIServiceRequest|OpenAPIConfigMapRequest|OpenAPIJobRequest" backend/api/openapi.yaml`
Expected: service/configmap schemas exist, simplified job schemas exist, no public YAML job input remains.

**Step 5: Commit**

```bash
git add backend/api/openapi.yaml
git commit -m "docs: update openapi resource schemas"
```

### Task 11: Update architecture and user-facing docs

**Files:**
- Modify: `docs/ARCHITECTURE.md`
- Modify: `docs/PROJECT_SUMMARY.md`
- Modify: `README.md`
- Test: `docs/ARCHITECTURE.md`

**Step 1: Write the failing test**

Use documentation review as the failing check.

Check whether docs still describe:

- job YAML passthrough
- no service/configmap OpenAPI support

**Step 2: Run verification to show stale docs**

Run: `rg -n "YAML|jobs|services|configmaps|/api/open" docs/ARCHITECTURE.md docs/PROJECT_SUMMARY.md README.md`
Expected: references show outdated OpenAPI behavior.

**Step 3: Write minimal implementation**

Update docs to explain:

- simplified resource models
- new service/configmap endpoints
- simplified job model
- ownership and namespace rules

**Step 4: Run verification to confirm**

Run: `rg -n "simplified|/api/open/services|/api/open/configmaps|ownerUser" docs/ARCHITECTURE.md docs/PROJECT_SUMMARY.md README.md`
Expected: docs mention the new contract.

**Step 5: Commit**

```bash
git add docs/ARCHITECTURE.md docs/PROJECT_SUMMARY.md README.md
git commit -m "docs: describe simplified openapi resources"
```

### Task 12: Run focused and broad verification

**Files:**
- Test: `backend/internal/models/...`
- Test: `backend/internal/handlers/...`
- Test: `backend/internal/k8s/...`

**Step 1: Write the failing test**

This task uses the verification suite produced by previous tasks.

**Step 2: Run focused verification**

Run: `go test ./backend/internal/models ./backend/internal/handlers ./backend/internal/k8s -v`
Expected: PASS

**Step 3: Run broader verification**

Run: `go test ./backend/... -v`
Expected: PASS

**Step 4: Run OpenAPI spec sanity check**

Run: `rg -n "/api/open/services|/api/open/configmaps|OpenAPIJobRequest" backend/api/openapi.yaml`
Expected: PASS with matching lines.

**Step 5: Commit**

```bash
git add backend api docs
git commit -m "test: verify simplified openapi resources"
```
