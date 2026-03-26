> **TL;DR:** `comp/core/secrets` resolves `ENC[handle]` placeholders in agent configuration by invoking a user-supplied executable, and supports periodic refresh, change notifications, Kubernetes scoping, audit logging, and automatic scrubbing of resolved values from logs and flares.

# comp/core/secrets — Secret Backend Integration Component

**Team:** agent-configuration
**Import path (interface):** `github.com/DataDog/datadog-agent/comp/core/secrets/def`
**Import path (fx wiring):** `github.com/DataDog/datadog-agent/comp/core/secrets/fx`

## Purpose

The secrets component allows sensitive values in agent configuration files to
be stored as opaque handles rather than plaintext. A handle looks like
`ENC[my_secret_name]`. When the agent loads configuration it calls
`Resolve`, which invokes a user-provided executable (`secret_backend_command`)
and substitutes the returned plaintext values in-memory. The original files on
disk are never modified.

The component also supports:

- **Periodic refresh** — re-invoking the backend at a configurable interval so
  that rotated secrets propagate to the running agent without a restart.
- **On-demand refresh** — triggered by an HTTP endpoint (`GET /secret/refresh`)
  or by the forwarder when an API key is rejected.
- **Change notifications** — subscribers are called whenever a secret value
  changes, allowing components to update live (e.g. swapping API keys in the
  forwarder).
- **Kubernetes scoping** — optional restrictions that limit which container
  images or Kubernetes namespaces can access which secrets.
- **Audit log** — refreshed secret handles (with scrubbed API/app key values)
  are appended to a rotating NDJSON file for compliance.
- **Status and flare integration** — a status page and flare entry expose the
  backend command path, permission check results, all known handles and where
  they appear in config, and any unresolved secrets.

## Key elements

### Key interfaces

#### Component interface (`def/component.go`)

```go
type Component interface {
    Configure(config ConfigParams)
    Resolve(data []byte, origin string, imageName string, kubeNamespace string, notify bool) ([]byte, error)
    SubscribeToChanges(callback SecretChangeCallback)
    Refresh() bool
    RefreshNow() (string, error)
    IsValueFromSecret(value string) bool
    RemoveOrigin(origin string)
}
```

| Method | Description |
|---|---|
| `Configure` | Initializes the backend (command path, timeout, refresh interval, K8s scoping rules, etc.). Must be called before `Resolve`. Starts the background refresh goroutine if a refresh interval is set. |
| `Resolve` | Walks the provided YAML bytes, replaces `ENC[handle]` values with their resolved secret, and returns the modified YAML. Uses an in-memory cache; only calls the backend for handles not yet cached. Set `notify=true` when the caller wants subscribers notified (required when config replaces values in memory). |
| `SubscribeToChanges` | Registers a `SecretChangeCallback` that is called whenever a handle's value changes. Fired both during `Resolve` and during periodic refresh. |
| `Refresh` | Non-blocking: enqueues an asynchronous refresh request (throttled by `apiKeyFailureRefreshInterval`). Returns `true` if the refresh mechanism is enabled. |
| `RefreshNow` | Blocking: immediately re-invokes the backend for all cached handles that match the refresh allowlist. Returns a human-readable summary string. |
| `IsValueFromSecret` | Returns `true` if the given plaintext string was ever returned by the backend — used by the scrubber to redact secret values from logs. |
| `RemoveOrigin` | Removes all handle-to-location mappings for a given config origin. Used when an integration config is removed from autodiscovery. |

### Key types

#### ConfigParams (`def/component.go`)

Key fields:

| Field | Description |
|---|---|
| `Command` | Path to the secret backend executable. Takes precedence over `Type`. |
| `Type` | Use the built-in `secret-generic-connector` binary for this backend type (sets `Command` automatically). |
| `Config` | Arbitrary key/value config passed to the backend in the JSON payload (used with `Type`). |
| `Arguments` | Extra CLI arguments appended to the backend command invocation. |
| `Timeout` | Backend command timeout in seconds. |
| `MaxSize` | Maximum allowed size (bytes) of the backend's stdout. |
| `RefreshInterval` | Seconds between periodic refreshes. `0` disables periodic refresh. |
| `RefreshIntervalScatter` | If true, the first refresh is scattered randomly within the interval to avoid thundering herds. |
| `GroupExecPerm` | If true, relax the executable permission check to allow group execution (useful in containers). |
| `RemoveLinebreak` | Strip trailing `\r\n` from resolved secret values. |
| `ScopeIntegrationToNamespace` | K8s: containers can only access secrets from their own namespace. |
| `AllowedNamespace` | K8s: explicit list of namespaces whose secrets are accessible. |
| `ImageToHandle` | K8s: per-image allowlist mapping image names to the handles they may access. |
| `APIKeyFailureRefreshInterval` | Minutes to wait between throttled refreshes triggered by an invalid API key. |
| `AuditFileMaxSize` | Maximum size (bytes) of the audit file before rotation. |
| `RunPath` | Directory where the audit file (`secret-audit-file.json`) is written. |

#### SecretChangeCallback (`def/type.go`)

```go
type SecretChangeCallback func(handle, origin string, path []string, oldValue, newValue any)
```

Called once per unique (handle, origin, path) tuple when a secret is first
resolved or when its value changes during a refresh. `path` is the YAML key
path where the handle appears (e.g. `["api_key"]` or
`["logs_config", "additional_endpoints", "0", "api_key"]`).

### Key functions

#### Refresh allowlist (`impl/secrets.go`)

Not all secrets are refreshed. The implementation applies an allowlist to
prevent partial in-memory config updates from causing inconsistent state:

- Secrets from integration configs (any origin other than `datadog.yaml`,
  `system-probe.yaml`, `security-agent.yaml`) are always refreshable.
- Secrets from agent config files are only refreshable if their YAML path
  contains one of: `api_key`, `app_key`, `additional_endpoints`,
  `orchestrator_additional_endpoints`, `profiling_additional_endpoints`,
  `debugger_additional_endpoints`, `debugger_diagnostics_additional_endpoints`,
  `symdb_additional_endpoints`.

#### Backend protocol

The component invokes `secret_backend_command` with a JSON payload on stdin:

```json
{
  "version": "1.1",
  "secrets": ["handle1", "handle2"],
  "secret_backend_timeout": 30
}
```

The backend must write a JSON object to stdout mapping each handle to a
`SecretVal`:

```json
{
  "handle1": {"value": "plaintext-value"},
  "handle2": {"error": "optional error message"}
}
```

On non-Windows platforms the backend executable must be owned by root and not
world-writable. Use `GroupExecPerm` to relax the group-execution check when
running in containers.

### Configuration and build flags

#### fx module (`fx/fx.go`)

`fx.Module()` provides `NewComponent`, which satisfies:

- `secrets.Component` — the main interface.
- `flaretypes.Provider` — populates `secrets.log` in flares.
- `api.AgentEndpointProvider` for `GET /secrets` — debug info endpoint.
- `api.AgentEndpointProvider` for `GET /secret/refresh` — manual refresh endpoint.
- `status.InformationProvider` — secrets section in `agent status`.

A no-op implementation is available at `comp/core/secrets/fx-noop` for builds
that do not need secrets support.

#### Mock (`mock/mock.go`)

`mock.New(t)` returns a `*Mock` that resolves handles from a map set via
`SetSecrets(map[string]string)`. Additional hook setters (`SetRefreshHook`,
`SetRefreshNowHook`, `SetIsValueFromSecretHook`) allow fine-grained control in
tests. `Configure` is a no-op in the mock.

## Usage

### Resolving secrets during configuration loading

```go
// pkg/config/setup/config.go — how the agent resolves its own config:
resolved, err := secretResolver.Resolve(yamlConf, origin, "", "", true)
```

The `origin` string is the config file name (e.g. `"datadog.yaml"`). The agent
configuration loader calls this for every config file it processes.

### Subscribing to secret changes

```go
secrets.SubscribeToChanges(func(handle, origin string, path []string, old, new any) {
    if slices.Contains(path, "api_key") {
        forwarder.UpdateAPIKey(new.(string))
    }
})
```

The forwarder health checker (`comp/forwarder/defaultforwarder/forwarder_health.go`)
uses this pattern to hot-swap API keys when they are rotated.

### Triggering a refresh after an API key failure

```go
// Called by the forwarder when a 403 is received from the intake:
if secrets.Refresh() {
    log.Info("scheduled secret refresh due to invalid API key")
}
```

### Checking if a log value came from a secret

```go
// Used by the scrubber before emitting log lines:
if secrets.IsValueFromSecret(candidateValue) {
    candidateValue = scrubber.Redact(candidateValue)
}
```

### Where it is wired in the agent

- `cmd/agent/subcommands/run/command.go` — core agent daemon.
- `cmd/security-agent`, `cmd/system-probe` — security and system probe.
- `comp/forwarder/defaultforwarder` — uses `secrets.Component` for key
  rotation and scrubbing.
- `comp/trace/agent/impl` — trace agent wires secrets for API key handling.
- `pkg/config/setup/config.go` — all configuration loading passes through
  `Resolve` before the config values are used.

---

## Related packages

- `comp/core/config` — calls `secrets.Component.Resolve` for every configuration file it loads. The config component receives `secrets.Component` via FX injection; the secrets component must therefore be wired before the config component initialises (both are part of `core.Bundle()`). See [comp/core/config docs](config.md).
- `pkg/util/scrubber` — after the secrets backend resolves a handle, the plaintext value is registered with the scrubber via `scrubber.AddStrippedKeys`. This ensures the resolved secret is redacted from all subsequent log lines and from the flare archive. `secrets.IsValueFromSecret` is called by the scrubber integration layer to identify values to redact. See [scrubber docs](../../pkg/util/scrubber.md).
- `comp/core/flare` — the secrets fx module self-registers a flare provider that adds `secrets.log` to every flare. The log contains the backend command path, permission check results, all known `ENC[...]` handles and the config paths where they appear, and any handles that failed to resolve. Resolved plaintext values are never written to the flare. See [comp/core/flare docs](flare.md).
