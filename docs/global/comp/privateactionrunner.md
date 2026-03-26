> **TL;DR:** `comp/privateactionrunner/impl` is the fx wiring layer for the Private Action Runner, handling lifecycle, self-enrollment, and configuration persistence while delegating action execution to `pkg/privateactionrunner`.

# comp/privateactionrunner/impl

**Package:** `github.com/DataDog/datadog-agent/comp/privateactionrunner/impl`
**Team:** action-platform

## Purpose

`comp/privateactionrunner/impl` is the component wiring layer for the **Private Action Runner (PAR)**. It bridges the agent's fx dependency-injection framework to the business logic in `pkg/privateactionrunner/`, managing lifecycle (start/stop), configuration resolution, and self-enrollment on first run.

The component is the entry point used when the agent binary starts the PAR. It does not implement action execution directly — that lives in `pkg/privateactionrunner/`.

## Key elements

### Key interfaces

```go
// comp/privateactionrunner/def/component.go
type Component interface{}

var ErrNotEnabled = errors.New("private action runner is not enabled")
```

The `Component` interface is intentionally empty. The runner is started as a side effect via the fx lifecycle hook, not by callers holding a reference to the interface. `ErrNotEnabled` is returned by `NewComponent` when the feature flag is off; fx treats this as an optional component and continues startup.

### Key types

**`PrivateActionRunner`** struct:

Central struct created by `NewComponent` and `NewPrivateActionRunner`:

| Field | Purpose |
|---|---|
| `coreConfig` | Reads and writes `private_action_runner.*` config keys at runtime (e.g. to persist a freshly enrolled identity) |
| `hostnameGetter` | Resolves the agent hostname used as the runner name prefix during self-enrollment |
| `rcClient` | Remote Config client adapter; used by `KeysManager` to fetch Datadog's public signing keys |
| `workflowRunner` | `*runners.WorkflowRunner` — the main task-polling and execution engine |
| `commonRunner` | `*runners.CommonRunner` — the OPMS health-check loop |
| `startChan` / `startOnce` | Ensure `Start` runs exactly once and allow `Stop` to wait for startup to complete before tearing down |

### Configuration and build flags

| Key | Purpose |
|---|---|
| `private_action_runner.enabled` | Feature flag; component exits early if false |
| `private_action_runner.self_enroll` | When true, the runner registers itself with Datadog using `api_key` + `app_key` |
| `private_action_runner.private_key` | ECDSA private key PEM (read from config or written after enrollment) |
| `private_action_runner.urn` | Runner URN `urn:dd:runner:<region>:<org>:<id>` (read or written after enrollment) |

### Key functions

#### `NewComponent(reqs Requires) (Provides, error)`

The fx constructor. Dependencies injected:

- `config.Component` — agent config
- `log.Component` — structured logger
- `compdef.Lifecycle` — fx lifecycle to register `OnStart`/`OnStop` hooks
- `rcclient.Component` — Remote Config client
- `hostname.Component` — hostname resolver
- `tagger.Component` — used to attach tags when auto-creating connections after enrollment
- `traceroute.Component` — passed to `WorkflowRunner` for network-path actions
- `eventplatform.Component` — passed to `WorkflowRunner` for event-platform actions

### Startup sequence (`Start`)

1. Load (or generate) the runner identity from config/persisted file.
2. If identity is incomplete and `self_enroll` is true, call `enrollment.SelfEnroll`, persist the result, and (if an actions allowlist is configured) auto-create connections in Datadog.
3. Build `KeysManager`, `TaskVerifier`, and `opms.Client`.
4. Start `WorkflowRunner` and `CommonRunner`.

### Graceful shutdown (`Stop`)

1. Cancel the startup context so that `start()` aborts if still in progress.
2. Wait up to `maxStartupWaitTimeout` (15 s) for startup to finish.
3. Call `Stop` on `WorkflowRunner`, then `CommonRunner`.

### fx module: `comp/privateactionrunner/fx`

```go
// fx/fx.go
func Module() fxutil.Module {
    return fxutil.Component(
        fxutil.ProvideComponentConstructor(privateactionrunnerimpl.NewComponent),
        fxutil.ProvideOptional[privateactionrunner.Component](),
        fx.Invoke(func(_ privateactionrunner.Component) {}),
    )
}
```

The `fx.Invoke` forces the component to be instantiated even though nothing else depends on it. `ProvideOptional` means the app starts normally if `ErrNotEnabled` is returned.

## Usage

Include `fx.Module()` in an fx application:

```go
import parfx "github.com/DataDog/datadog-agent/comp/privateactionrunner/fx"

fx.New(
    parconfig.Module(),         // agent config
    parfx.Module(),             // private action runner
    ...
)
```

The runner starts automatically when the fx app starts and shuts down when the app stops. No additional calls are required.

For the business-logic details (action bundles, credential resolution, OPMS protocol, task verification), see [pkg/privateactionrunner](../pkg/privateactionrunner.md).

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/privateactionrunner`](../pkg/privateactionrunner.md) | The **business-logic layer**. `comp/privateactionrunner/impl` is the thin fx wiring layer that reads config keys, calls `enrollment.SelfEnroll` on first run, constructs `KeysManager` / `TaskVerifier` / `WorkflowRunner` / `CommonRunner`, and registers fx lifecycle hooks. All action bundles, credential resolution, OPMS protocol handling, and task verification live in `pkg/privateactionrunner`. |
| [`comp/remote-config/rcclient`](remote-config/rcclient.md) | `rcClient` is injected into `PrivateActionRunner` and passed to `KeysManager`. `KeysManager` subscribes to a Remote Config product via `rcclient.Subscribe` to receive Datadog's public signing keys over the TUF-verified RC channel. `KeysManager.WaitForReady()` blocks the `WorkflowRunner.Start` call until the first key set has been delivered, making the RC client an explicit startup dependency. If the RC client is not yet connected when the runner starts, task verification will block until keys arrive. |

### Startup dependency on Remote Config

Because `KeysManager` must wait for signing keys before `WorkflowRunner` can accept tasks, the overall startup sequence is:

```
fx OnStart
  └─ PrivateActionRunner.Start()
       ├─ load / enroll identity
       ├─ build KeysManager (subscribes to RC product via rcclient)
       ├─ KeysManager.WaitForReady()  ← blocks until RC delivers first key set
       ├─ WorkflowRunner.Start()
       └─ CommonRunner.Start()
```

If Remote Config is disabled (`remote_configuration.enabled: false`) or the RC service is unreachable, `KeysManager.WaitForReady()` will block until the startup context deadline (`maxStartupWaitTimeout` = 15 s), after which the runner aborts startup. Ensure Remote Config is reachable when deploying the Private Action Runner.
