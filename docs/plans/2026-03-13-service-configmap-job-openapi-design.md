# Service ConfigMap Job OpenAPI Design

## Background

Genet already exposes simplified OpenAPI support for Pods and Kubernetes-native YAML support for Jobs. The next iteration should make `Pod`, `Service`, `ConfigMap`, and `Job` follow a single Genet-style resource model:

- external callers send simplified JSON payloads
- the server derives owner namespace and management labels
- handlers convert simplified requests into Kubernetes-native resources
- responses return Genet-oriented views instead of raw Kubernetes objects

This design covers the target API contract, backend structure, update semantics, ownership rules, and rollout constraints.

## Goals

- Extend `/api/open/*` to support simplified CRUD for `Service` and `ConfigMap`.
- Replace current Job YAML passthrough with a simplified Genet Job model.
- Keep Pod OpenAPI behavior intact while aligning the resource family around the same style.
- Preserve ownership isolation through API key `ownerUser` binding and namespace derivation.
- Keep the API readable for SDK and frontend consumers without requiring Kubernetes expertise.

## Non-Goals

- No generic `/api/open/resources` endpoint.
- No raw Kubernetes passthrough for `Service`, `ConfigMap`, or `Job`.
- No new OpenAPI support for `Ingress`, `Secret`, `PVC`, `CronJob`, or arbitrary core resources.
- No user-controlled namespace or label ownership fields.

## Design Principles

### Resource-centric simplified models

Each resource keeps an independent request/response schema with fields chosen for Genet workflows rather than full Kubernetes parity.

### Server-owned tenancy

The server continues to derive `userIdentifier` from API key `ownerUser`, then maps it to `user-{identifier}`. Clients do not choose namespace. The server also injects ownership and management labels.

### Consistent OpenAPI surface

All four resources use split endpoints under `/api/open/<resource>` and share the same authentication and scope model.

### Genet responses, not raw Kubernetes objects

List and get endpoints return concise Genet DTOs tailored for callers. Raw Kubernetes status structures are hidden unless specifically flattened into response fields.

## API Surface

### Pod

Keep the existing simplified JSON contract and current CRUD endpoints:

- `POST /api/open/pods`
- `GET /api/open/pods`
- `GET /api/open/pods/{id}`
- `PUT /api/open/pods/{id}`
- `DELETE /api/open/pods/{id}`

`PUT` keeps recreate semantics.

### Service

Add:

- `POST /api/open/services`
- `GET /api/open/services`
- `GET /api/open/services/{name}`
- `PUT /api/open/services/{name}`
- `DELETE /api/open/services/{name}`

`Service` uses a simplified request model:

- `name`
- `type`: `ClusterIP | NodePort | LoadBalancer`
- `targetPodName`: optional convenience field; auto-builds selector `app=<podName>`
- `selector`: optional explicit selector map; mutually exclusive with `targetPodName`
- `ports`: array of service ports
- `sessionAffinity`
- `publishNotReadyAddresses`
- `annotations`

Service port item:

- `name`
- `protocol`
- `port`
- `targetPort`
- `nodePort`

### ConfigMap

Add:

- `POST /api/open/configmaps`
- `GET /api/open/configmaps`
- `GET /api/open/configmaps/{name}`
- `PUT /api/open/configmaps/{name}`
- `DELETE /api/open/configmaps/{name}`

`ConfigMap` uses a simplified request model:

- `name`
- `data`
- `binaryData` as base64 string map
- `immutable`
- `annotations`

### Job

Keep the same routes but replace YAML passthrough with a simplified JSON request model:

- `POST /api/open/jobs`
- `GET /api/open/jobs`
- `GET /api/open/jobs/{name}`
- `PUT /api/open/jobs/{name}`
- `DELETE /api/open/jobs/{name}`

`Job` request:

- `name`
- `image`
- `command`
- `args`
- `env`
- `workingDir`
- `gpuType`
- `gpuCount`
- `cpu`
- `memory`
- `shmSize`
- `nodeName`
- `gpuDevices`
- `userMounts`
- `parallelism`
- `completions`
- `backoffLimit`
- `ttlSecondsAfterFinished`
- `restartPolicy`
- `annotations`

## Ownership and Namespace Rules

All OpenAPI-managed resources must:

- resolve owner from API key `ownerUser`
- resolve namespace from `userIdentifier`
- set `genet.io/open-api=true`
- set `genet.io/managed=true`
- set `genet.io/openapi-owner=<ownerUser>`

Clients cannot provide namespace. User-supplied labels are not supported in v1 of this design because labels are part of server-side ownership and lifecycle management.

For reads and mutations, handlers must validate the owner label before returning or mutating a resource. Ownership mismatch should return `404` to avoid leaking resource existence.

## Update Semantics

### Pod

- `PUT` uses delete-then-recreate.
- Existing behavior stays unchanged.

### Service

- `PUT` uses in-place update.
- Preserve Kubernetes-managed fields where required.
- If immutable field changes cause Kubernetes rejection, return `409`.
- Avoid delete-recreate to preserve stable `clusterIP` and load balancer identity.

### ConfigMap

- `PUT` uses in-place update.
- If the existing ConfigMap is immutable, return `409`.

### Job

- `PUT` uses recreate semantics.
- If the existing Job is still active, return `409`.
- If completed or failed, delete and recreate using the simplified model.

## Backend Architecture

### Routing

Keep routes in `backend/cmd/api/main.go` under `/api/open`, aligned with current `pods` and `jobs`.

### Handler layout

Split the current large OpenAPI handler file into resource-focused files:

- `backend/internal/handlers/openapi_common.go`
- `backend/internal/handlers/openapi_pod.go`
- `backend/internal/handlers/openapi_service.go`
- `backend/internal/handlers/openapi_configmap.go`
- `backend/internal/handlers/openapi_job.go`

Shared helper responsibilities:

- apply owner context
- derive namespace
- inject standard labels
- append list selectors
- normalize Kubernetes errors into API responses

### Model layout

Add dedicated OpenAPI resource DTOs under `backend/internal/models`:

- `openapi_service.go`
- `openapi_configmap.go`
- `openapi_job.go`

Keep these separate from UI-facing Pod DTOs so the external contract remains explicit and versionable.

### Conversion layer

Introduce dedicated request-to-Kubernetes builders:

- `backend/internal/k8s/service_builder.go`
- `backend/internal/k8s/configmap_builder.go`
- `backend/internal/k8s/job_builder.go`

Builders should only transform validated simplified inputs into Kubernetes resources. They should not depend on Gin or HTTP concerns.

## Shared Runtime Builder for Pod and Job

The biggest structural change is to extract reusable workload runtime construction from `backend/internal/k8s/pod.go`.

Today Pod creation mixes:

- container resource setup
- proxy env injection
- downward API env injection
- GPU scheduling logic
- storage and mount assembly
- affinity and selector assembly
- wrapper creation as a Pod object

The design should split that into:

- a shared runtime builder that produces container, volumes, mounts, affinity, selector, and runtime-class decisions
- a Pod wrapper
- a Job template wrapper

This keeps `Pod` and simplified `Job` aligned on:

- GPU behavior
- mount behavior
- proxy behavior
- security context
- scheduler constraints

Without this split, Job support will drift from Pod support and become expensive to maintain.

## Response Models

### Service response

- `name`
- `namespace`
- `type`
- `selector`
- `ports`
- `clusterIP`
- `externalIPs`
- `loadBalancer`
- `createdAt`

### ConfigMap response

List response should use summary fields:

- `name`
- `namespace`
- `immutable`
- `dataKeys`
- `binaryDataKeys`
- `createdAt`

Get/create/update response should include full `data` and `binaryData`.

### Job response

- `name`
- `namespace`
- `image`
- `parallelism`
- `completions`
- `active`
- `succeeded`
- `failed`
- `status`
- `createdAt`
- `completionTime`

## Validation Rules

### Common

- `name` must pass Kubernetes DNS-1123 validation.
- user annotations may be allowed, but `genet.io/*` is reserved.
- namespace is forbidden in request payloads.

### Service

- `selector` and `targetPodName` are mutually exclusive.
- one of them must be present.
- `targetPodName` must reference an existing Pod in the owner namespace.
- `nodePort` allowed only for `NodePort` or `LoadBalancer` when Kubernetes permits it.

### ConfigMap

- at least one of `data` or `binaryData` should be present unless empty maps are explicitly allowed.
- `binaryData` values must be valid base64.

### Job

- `restartPolicy` limited to `Never` or `OnFailure`.
- `parallelism`, `completions`, `backoffLimit`, and `ttlSecondsAfterFinished` should have sanity bounds.
- resource validation should mirror existing Pod validation where applicable.

## Error Mapping

Use a consistent external error surface:

- `400` invalid request or validation failure
- `403` missing `ownerUser` binding or scope failure
- `404` resource not found or not owned by caller
- `409` already exists, immutable update conflict, active Job update, immutable field conflict
- `500` unexpected Kubernetes or server failure

Logs should preserve the raw Kubernetes error, but external responses should stay concise and stable.

## OpenAPI Documentation Changes

`backend/api/openapi.yaml` should be updated to:

- add `Service` and `ConfigMap` resource paths
- convert `Job` request/response examples from YAML/native schema to simplified JSON
- add shared reusable components for:
  - service ports
  - env vars
  - user mounts
  - list response wrappers
- clearly state server-owned namespace and labels in every resource description

## Testing Strategy

### Handler tests

- request validation
- ownership injection
- read/write scope enforcement
- conflict and not-found mapping

### Builder tests

- simplified request to Kubernetes object conversion
- default filling
- label and annotation injection
- reserved annotation blocking

### Integration tests

- CRUD flow for Service
- CRUD flow for ConfigMap
- CRUD flow for simplified Job
- owner label filtering and isolation
- Job update conflict when active
- ConfigMap immutable conflict
- Service `targetPodName` validation

### Regression coverage

Add tests that verify Pod and Job runtime builders stay aligned on:

- GPU resources and device env vars
- proxy env vars
- shared memory
- user mounts
- scheduling hints

## Rollout Plan

Recommended sequence:

1. finalize DTOs and OpenAPI schemas
2. extract Pod runtime builder shared by Pod and Job
3. implement ConfigMap simplified OpenAPI
4. implement Service simplified OpenAPI
5. migrate Job from YAML passthrough to simplified JSON
6. update docs and examples

This order reduces risk because Job has the largest compatibility and runtime impact.

## Compatibility Notes

The main breaking change is Job input format moving from YAML passthrough to simplified JSON. That should be treated as an explicit API contract change rather than silently mixed with the old mode.

Pod behavior remains compatible. Service and ConfigMap are additive.

## Final Recommendation

Adopt resource-specific Genet simplified models for `Pod`, `Service`, `ConfigMap`, and `Job`, with:

- server-controlled tenancy and labels
- concise Genet response DTOs
- in-place updates for `Service` and `ConfigMap`
- recreate updates for `Pod` and `Job`
- a shared runtime builder to keep Pod and Job behavior consistent
