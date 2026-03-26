# pkg/privateactionrunner

## Purpose

`pkg/privateactionrunner` implements the **Private Action Runner (PAR)**: a component of the Datadog Agent that executes Datadog Workflow actions inside private networks. It bridges Datadog's cloud-hosted workflow engine with resources (Kubernetes clusters, GitLab, Jenkins, databases, scripts, …) that are not reachable from the public internet.

Key responsibilities:
- **Enrollment**: Register the runner with Datadog (ECDSA key-pair generation, API key-based self-enrollment).
- **Task polling**: Long-poll the Datadog On-Prem Management Service (OPMS) for workflow tasks; verify cryptographic signatures; execute actions; publish success or failure results.
- **Action dispatch**: Map task FQNs (`<bundle_id>.<action_name>`) to concrete implementations via a registry of action bundles.
- **Credential resolution**: Resolve connection credentials (token auth, basic auth) from Docker secrets or plain-text values at execution time.
- **Observability**: Emit StatsD metrics, structured logs, and heartbeats for in-flight tasks.

The binary entry point is `cmd/privateactionrunner/`.

---

## Key Elements

### Types (`types/`)

| Symbol | Kind | Description |
|--------|------|-------------|
| `Bundle` | interface | A named collection of actions. Single method: `GetAction(name) Action`. |
| `Action` | interface | Single method: `Run(ctx, *Task, *PrivateCredentials) (interface{}, error)`. |
| `Task` | struct | A dequeued workflow task. Carries a task ID, `Attributes` (bundle ID, action name, inputs, org ID, job ID, signed envelope, connection info), and the raw wire bytes. |
| `Attributes` | struct | Task metadata including `Inputs map[string]interface{}` (action-specific parameters) and `SignedEnvelope` for signature verification. |
| `ExtractInputs[T](task)` | func | Generic helper that marshals `task.Data.Attributes.Inputs` to JSON and unmarshals into `T`. Used by every action to parse its parameters. |
| `Task.GetFQN()` | method | Returns `"<bundle_id>.<action_name>"`. |
| `Task.Validate()` | method | Checks that the task is non-nil and has a JobId. |

### Runners (`runners/`)

| Symbol | Kind | Description |
|--------|------|-------------|
| `Runner` | interface | `Start(ctx) error` / `Stop(ctx) error`. |
| `CommonRunner` | struct | Handles the OPMS health-check loop. Decouples its loop context from the startup context so it survives the startup deadline. |
| `WorkflowRunner` | struct | Main execution engine. Owns the bundle registry, OPMS client, credential resolver, task verifier, and the polling loop. |
| `NewWorkflowRunner(cfg, keysManager, verifier, opmsClient, traceroute, eventPlatform)` | func | Constructor. |
| `WorkflowRunner.RunTask(ctx, task, creds)` | method | Looks up the bundle and action from the FQN, enforces the action allowlist and URL allowlist, runs the action, and publishes the result. Sends heartbeats in a background goroutine. |
| `Loop` | struct | The polling loop inside `WorkflowRunner`. Dequeues tasks from OPMS, verifies signatures, resolves credentials, and dispatches to a goroutine pool of size `config.RunnerPoolSize`. Uses a circuit breaker for backoff on consecutive dequeue failures. |

### Bundles (`bundles/`)

`bundles/registry.go` (selected by build tag; `registry_kubeapiserver.go` for the Kubernetes API server variant) maps bundle IDs to `Bundle` implementations:

| Bundle ID | Implementation | Notes |
|-----------|---------------|-------|
| `com.datadoghq.ddagent` | `bundles/ddagent` | Agent self-actions |
| `com.datadoghq.ddagent.networkpath` | `bundles/ddagent/networkpath` | Traceroute-style path actions |
| `com.datadoghq.gitlab.*` | `bundles/gitlab/*` | ~18 GitLab API sub-bundles (branches, pipelines, MRs, etc.) |
| `com.datadoghq.http` | `bundles/http` | Generic HTTP requests (URL allowlist enforced) |
| `com.datadoghq.jenkins` | `bundles/jenkins` | Jenkins API |
| `com.datadoghq.kubernetes.*` | `bundles/kubernetes/*` | 6 Kubernetes API groups (core, apps, batch, etc.) |
| `com.datadoghq.mongodb` | `bundles/mongodb` | MongoDB operations |
| `com.datadoghq.remoteaction.rshell` | `bundles/remoteaction/rshell` | Restricted shell (path allowlist enforced) |
| `com.datadoghq.script` | `bundles/script` | Script execution |
| `com.datadoghq.temporal` | `bundles/temporal` | Temporal workflow signals |

`NewRegistry(cfg, traceroute, eventPlatform)` instantiates all bundles and returns a `*Registry`. `Registry.GetBundle(id)` returns the named bundle (or `nil`).

### Config (`adapters/config/`)

`Config` is the central configuration struct, populated from the agent config YAML and passed to all major components.

Key fields:

| Field | Type | Purpose |
|-------|------|---------|
| `ActionsAllowlist` | `map[string]sets.Set[string]` | Per-bundle set of allowed action names (`"*"` for all). |
| `Allowlist` | `[]string` | Hostname glob patterns allowed for HTTP bundle requests. |
| `RShellAllowedPaths` | `[]string` | Filesystem paths allowed for rshell actions. |
| `PrivateKey` | `*ecdsa.PrivateKey` | Runner identity key used to sign OPMS requests. |
| `Urn` | `string` | Runner URN (`urn:dd:runner:<region>:<org>:<id>`). |
| `OrgId` | `int64` | Org ID; checked against task org ID. |
| `RunnerPoolSize` | `int32` | Max concurrent in-flight tasks. |
| `TaskTimeoutSeconds` | `*int32` | Per-task execution deadline. |
| `HeartbeatInterval` | `time.Duration` | Interval between heartbeat calls for in-flight tasks. |

`Config.IsActionAllowed(bundleId, actionName)` and `Config.IsURLInAllowlist(url)` are the two security enforcement points called before every action.

### Credentials (`credentials/`)

`credentials/resolver/PrivateCredentialResolver` resolves a `ConnectionInfo` proto (from the task) into a `*privateconnection.PrivateCredentials` struct that bundles use for outbound authentication.

Two supported auth types:

| Type | Credential source |
|------|-------------------|
| `Token Auth` | Plain-text token, Docker secret file (`/run/secrets/…`), or YAML file |
| `Basic Auth` | Username (plain-text) + password from Docker secret file |

Docker secret files are JSON with the format:
```json
{ "auth_type": "Token Auth", "credentials": [{ "tokenName": "api-key", "tokenValue": "…" }] }
```

File size is capped at 1 MB.

### Enrollment (`enrollment/`)

`SelfEnroll(ctx, ddSite, runnerNamePrefix, hostname, apiKey, appKey)` performs first-time runner registration:
1. Generates an ECDSA key pair.
2. Calls `opms.PublicClient.EnrollWithApiKey` to register on the Datadog API.
3. Returns a `Result` containing the private key, runner URN, hostname, and runner name.

The identity (private key + URN) is persisted to `privateactionrunner_private_identity.json` and loaded on subsequent starts.

### Task verification (`task-verifier/`)

`TaskVerifier.UnwrapTaskFromSignedEnvelope(envelope)` verifies a task's cryptographic signature before execution:
- Checks that the envelope is non-empty and carries at least one signature.
- Verifies SHA-256 hash and ECDSA signature against keys fetched via `KeysManager`.
- Validates expiration timestamp.
- Checks that the task's org ID matches the runner's configured org ID.

`KeysManager` fetches and caches Datadog's public signing keys (via Remote Config), and `WaitForReady()` blocks `WorkflowRunner.Start` until keys are available.

### OPMS client (`opms/`)

`opms.Client` wraps the OPMS HTTP API:

| Method | Endpoint | Purpose |
|--------|----------|---------|
| `DequeueTask` | `POST /workflow-tasks/dequeue` | Long-poll for the next task |
| `PublishSuccess` | `POST /workflow-tasks/publish-task-update` | Report action output |
| `PublishFailure` | `POST /workflow-tasks/publish-task-update` | Report error code and message |
| `Heartbeat` | `POST /workflow-tasks/heartbeat` | Keep task alive |
| `HealthCheck` | `GET /runner/health-check` | Liveness check |

Requests are signed with the runner's ECDSA private key. The client is shared between `CommonRunner` (health check) and `WorkflowRunner` (task polling).

### Modes (`adapters/modes/`)

`Mode` is a string type. Currently only `ModePull` (`"pull"`) is defined, meaning the runner polls OPMS rather than having tasks pushed to it.

---

## Usage

### How a task flows through the system

```
OPMS ──── DequeueTask ────► Loop.Run()
                                │
                          validate + verify signature (TaskVerifier)
                                │
                          resolve credentials (PrivateCredentialResolver)
                                │
                          WorkflowRunner.RunTask()
                           ├── lookup Bundle in Registry
                           ├── check ActionsAllowlist
                           ├── (HTTP bundle) check URL allowlist
                           ├── start heartbeat goroutine
                           └── Action.Run(ctx, task, creds)
                                │
                          PublishSuccess / PublishFailure ──► OPMS
```

### Implementing a new action bundle

1. Create `pkg/privateactionrunner/bundles/<name>/` with one or more `Action` implementations.
2. Each action implements `Run(ctx, *types.Task, *privateconnection.PrivateCredentials) (interface{}, error)`. Use `types.ExtractInputs[MyInputs](task)` to parse inputs.
3. Implement `Bundle.GetAction(name) types.Action`.
4. Register the bundle in `bundles/registry.go` with a stable FQN string like `"com.datadoghq.<name>"`.

### Security enforcement

Two layers of policy are checked in `WorkflowRunner.RunTask` before every action executes:
- **Action allowlist**: `config.IsActionAllowed(bundleId, actionName)` — must be configured explicitly per action.
- **URL allowlist**: `config.IsURLInAllowlist(url)` — applied only to `com.datadoghq.http`; supports glob patterns.

Tasks that fail allowlist checks receive a `PublishFailure` response with a structured error code, not a process-level error.

### Key importers

The package is consumed by the agent binary (`cmd/privateactionrunner/`) and by the component wiring in `comp/` (54 importers across the codebase), including the agent startup sequence that calls `enrollment.SelfEnroll` when no persisted identity exists and then starts `CommonRunner` and `WorkflowRunner`.

---

## Cross-references

| Topic | See also |
|-------|----------|
| fx component wiring (lifecycle, enrollment, config keys) | [`comp/privateactionrunner/impl`](../comp/privateactionrunner.md) |
| Remote Config client used by `KeysManager` to fetch public signing keys | [`pkg/remoteconfig`](remoteconfig.md) |
| Network-path traceroute used by the `com.datadoghq.ddagent.networkpath` bundle | [`pkg/networkpath`](networkpath.md) |

### Relationship to `comp/privateactionrunner`

`pkg/privateactionrunner` is the **business-logic layer**: action bundles, credential resolution, OPMS protocol, task verification. `comp/privateactionrunner/impl` (see [`comp/privateactionrunner`](../comp/privateactionrunner.md)) is the **wiring layer**: it reads config, calls `enrollment.SelfEnroll` on first run, constructs `KeysManager` / `TaskVerifier` / `WorkflowRunner`, and registers `OnStart` / `OnStop` fx lifecycle hooks.

### How `KeysManager` uses Remote Config

`KeysManager` subscribes to a product on the RC client (`rcclient.Component`) to receive Datadog's public signing keys. The keys are delivered via the standard `pkg/remoteconfig/state` TUF-verified update path (see [`pkg/remoteconfig`](remoteconfig.md)). `KeysManager.WaitForReady()` blocks `WorkflowRunner.Start` until the first key set has been delivered; this is why `rcClient` is an explicit dependency of `comp/privateactionrunner/impl`.

### How the `networkpath` bundle integrates `pkg/networkpath`

The `com.datadoghq.ddagent.networkpath` bundle wraps `comp/networkpath/traceroute` (the traceroute component) and calls `Runner.Run(ctx, config.Config)` from [`pkg/networkpath`](networkpath.md). The `traceroute.Component` is injected into `WorkflowRunner` at construction time via `NewWorkflowRunner(..., traceroute, ...)` and forwarded to `NewRegistry(cfg, traceroute, eventPlatform)`.

```
WorkflowRunner.RunTask(ctx, task, creds)
    │  FQN: "com.datadoghq.ddagent.networkpath.<action>"
    ▼
bundles/ddagent/networkpath.Action.Run(ctx, task, creds)
    │  types.ExtractInputs[NetworkPathInputs](task)
    ▼
comp/networkpath/traceroute.Component.Run(ctx, config.Config)
    │  (uses pkg/networkpath/traceroute/runner internally)
    ▼
payload.NetworkPath  →  eventplatform.Component (event platform forwarder)
```
