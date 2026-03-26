> **TL;DR:** Authoritative registry for all agent configuration keys and their defaults — owns the two global `Config` singletons (`Datadog`, `SystemProbe`), the `InitConfig`/`LoadDatadog` startup sequence, and every `BindEnvAndSetDefault` call that defines the agent's configuration schema.

# pkg/config/setup

**Import path:** `github.com/DataDog/datadog-agent/pkg/config/setup`

## Purpose

`pkg/config/setup` is the authoritative registry for all agent configuration keys and their defaults. It owns two global `pkgconfigmodel.Config` singletons — `Datadog` (for `datadog.yaml`) and `SystemProbe` (for `system-probe.yaml`) — and provides the functions that populate their schemas via `BindEnvAndSetDefault` calls.

Every part of the agent that needs to read configuration either calls `setup.Datadog()` directly (for legacy or cross-cutting code) or receives a `pkgconfigmodel.Reader`/`pkgconfigmodel.Config` that was ultimately initialized by this package. With over 1 000 call-sites using `setup.Datadog()` alone, this package is the single source of truth for what configuration keys exist, what their defaults are, and what environment variable overrides apply.

**Related packages:**
- [`pkg/config/model`](model.md) — interfaces (`Reader`, `Writer`, `Config`, `BuildableConfig`) and shared types (`Source`, `Proxy`, `Warnings`) consumed throughout
- [`pkg/config`](config.md) — overview of all config sub-packages and how they relate
- [`pkg/config/env`](env.md) — `DetectFeatures` is called by `LoadDatadog` and populates the feature map used by autodiscovery
- [`comp/core/config`](../../comp/core/config.md) — the fx-injectable component that wraps this package's globals and injects `model.Reader` into components

## Key Elements

### Key functions

### Global accessors

```go
func Datadog() pkgconfigmodel.Config
func SystemProbe() pkgconfigmodel.Config
```

Return the live global singletons. `Datadog()` is the config object read from `datadog.yaml`; `SystemProbe()` from `system-probe.yaml`. Both are initialized during `init()` (via `initConfig()`) and populated by `InitConfig` / `InitSystemProbeConfig`. In tests a separate `test`-build-tagged version is used (see the `config_test_accessor.go` file), which allows `mock.New(t)` to replace the singleton for the duration of a test.

```go
func GlobalConfigBuilder() pkgconfigmodel.BuildableConfig
func GlobalSystemProbeConfigBuilder() pkgconfigmodel.BuildableConfig
```

Expose the underlying `BuildableConfig` interface for the few places that need to construct or replace the config object (e.g., the component framework's config loader). Avoid calling these outside of startup code.

### Initialization

```go
func InitConfigObjects(cliPath string, defaultDir string)
```

Creates both global config objects, registers all key defaults, and calls `BuildSchema()`. Called once from `main` for every agent binary. It also inspects an optional `conf_nodetreemodel` field in `datadog.yaml` to decide which backing config library to use. Safe to call multiple times (recreates singletons each time), which allows tests to start from a clean state.

```go
func InitConfig(config pkgconfigmodel.Setup)
func InitSystemProbeConfig(config pkgconfigmodel.Setup)
```

Register all configuration keys and defaults for the core agent and for system-probe, respectively. `InitConfig` is split between `initCommonConfigComponents` (keys shared with serverless) and `initCoreAgentFull` (core-agent-only keys). Every subsystem registers its own keys via a private `init*` function called from these entry points — for example `setupAPM`, `logsagent`, `dogstatsd`, `kubernetes`, etc.

Under the `serverless` build tag, `config_init_serverless.go` provides an alternative `initConfig()` that calls only `initCommonConfigComponents`, omitting core-agent-only keys to keep the schema small.

To add a new config key, call `config.BindEnvAndSetDefault` inside the appropriate subsystem function in `common_settings.go` (for keys reachable by serverless) or directly in `initCoreAgentFull` (for core-agent-only keys). Do **not** add calls to `config.go`.

### Loading config at runtime

```go
func LoadDatadog(
    config pkgconfigmodel.Config,
    secretResolver secrets.Component,
    delegatedAuthComp delegatedauth.Component,
    additionalEnvVars []string,
) error

func LoadSystemProbe(config pkgconfigmodel.Config, additionalKnownEnvVars []string) error
```

`LoadDatadog` orchestrates the full config lifecycle after initialization:
1. Reads `datadog.yaml` from disk (`ReadInConfig`).
2. Warns on unknown keys or unexpected Unicode.
3. Resolves proxy settings from environment variables (`LoadProxyFromEnv`).
4. Decrypts secrets via the secrets backend.
5. Configures delegated auth (cloud-provider-sourced API keys).
6. Calls `pkgconfigenv.DetectFeatures` (so feature flags are always set, even if the file is missing).
7. Applies any registered override functions (`model.ApplyOverrideFuncs`).

`LoadDatadog` is called by `comp/core/config` during the component's `OnStart` hook. Code that runs before the component starts should not call `LoadDatadog` directly; instead, use `comp/core/config` and express the ordering dependency through fx.

### Proxy loading

```go
func LoadProxyFromEnv(config pkgconfigmodel.ReaderWriter)
```

Reads `DD_PROXY_HTTP`, `DD_PROXY_HTTPS`, `DD_PROXY_NO_PROXY` (and the lowercase variants `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) and writes them back into the config. Called by `LoadDatadog` before secret resolution so that secrets can themselves be fetched through a proxy.

### IPC helpers

```go
func GetIPCAddress(config pkgconfigmodel.Reader) (string, error)
func GetIPCPort() string
```

Return the address and port that the agent's IPC/CMD endpoint listens on. `GetIPCAddress` validates that the address is a local address and handles the deprecated `ipc_address` → `cmd_host` migration.

### Config merging

```go
func Merge(configPaths []string, config pkgconfigmodel.Config) error
```

Merges one or more additional YAML files into an existing config object. Used by the fleet policies feature and extra config paths.

### Key types

### Structs for complex config keys

| Type | Config key | Description |
|---|---|---|
| `ConfigurationProviders` | `config_providers` | Autodiscovery config providers (polling, templates, auth) |
| `Listeners` | `listeners` | Autodiscovery listeners; exposes `IsProviderEnabled` |

### Configuration and build flags

### Platform-specific path constants

Declared in `config_nix.go`, `config_windows.go`, and `config_darwin.go`, these are exported so other packages can reference default paths without hard-coding them:

| Constant / Variable | Value (Linux) |
|---|---|
| `InstallPath` | `/opt/datadog-agent` (derived from executable location at runtime) |
| `DefaultDDAgentBin` | `<InstallPath>/bin/agent` |
| `DefaultSystemProbeAddress` | `<InstallPath>/run/sysprobe.sock` |
| `DefaultProcessAgentLogFile` | `/var/log/datadog/process-agent.log` |
| `DefaultSecurityAgentLogFile` | `/var/log/datadog/security-agent.log` |
| `DefaultUpdaterLogFile` | `/var/log/datadog/updater.log` |

### Notable numeric/string constants

These are exported because they appear in multiple packages:

| Constant | Value | Meaning |
|---|---|---|
| `DefaultSite` | `"datadoghq.com"` | Default Datadog intake site |
| `DefaultNumWorkers` / `MaxNumWorkers` | 4 / 25 | Check runner worker pool |
| `DefaultBatchMaxSize` | 1000 | Max log events per HTTP batch |
| `DefaultBatchMaxContentSize` | 5 000 000 | Max log batch size in bytes |
| `DefaultCompressorKind` | `"zstd"` | Default payload compressor |
| `DefaultFingerprintStrategy` | `"disabled"` | Log file fingerprinting strategy |

### Other exported values

| Variable | Description |
|---|---|
| `StandardJMXIntegrations` | Map of integration names known to be JMXFetch-based. Deprecated — new integrations should use `is_jmx: true` in their config instead. |
| `StandardStatsdPrefixes` | Slice of statsd metric prefixes used by the agent and its components (`datadog.agent`, `datadog.dogstatsd`, etc.) |

### `constants` sub-package

`pkg/config/setup/constants` holds a small set of cache-key strings (`ClusterIDCacheKey`, `NodeKubeDistributionKey`, `ECSClusterMetaCacheKey`) and `DefaultEBPFLessProbeAddr` that are consumed by both the agent and system-probe without pulling in the full `setup` package.

### Test helpers (build tag `test`)

```go
func NewChangeChecker() *ChangeChecker   // snapshot the config; HasChanged() detects mutations
```

`ChangeChecker` is designed for `TestMain`: snapshot the global config before tests run, then assert it was not mutated. Build-tagged (`//go:build test`) so it is only compiled in test binaries.

The `test_helpers.go` file exposes `newTestConf(t)` (calls `InitConfig` on a fresh object) and `newEmptyMockConf(t)` (raw empty config with dynamic schema) for in-package tests. External tests should use `pkg/config/mock` instead.

## Usage

### Reading a config value

The vast majority of the codebase does one of the following:

```go
// Legacy pattern — reads from the global singleton
import pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

value := pkgconfigsetup.Datadog().GetString("api_key")
enabled := pkgconfigsetup.Datadog().GetBool("logs_enabled")
```

```go
// Preferred pattern — accept a model.Reader via dependency injection
func NewFoo(cfg pkgconfigmodel.Reader) *Foo {
    return &Foo{endpoint: cfg.GetString("dd_url")}
}
```

The component framework (`comp/core/config`) wraps the global config and injects `model.Reader` into components, so new code should prefer the injected form over calling `setup.Datadog()` directly. This also makes components easier to test with `config.NewMock(t)`.

### Adding a new config key

Add a `BindEnvAndSetDefault` call to the appropriate section of `common_settings.go` or a subsystem file (e.g., `apm.go`, `system_probe.go`). The method signature is:

```go
config.BindEnvAndSetDefault("my_feature.my_key", defaultValue, "DD_MY_FEATURE_MY_KEY")
```

The third argument (env var override) is optional; if omitted the config layer derives it automatically as `DD_` + uppercased key. See [`pkg/config/model`](model.md) for the full `BindEnvAndSetDefault` interface and source-priority rules.

### Agent startup sequence

```go
// In main (or equivalent entry point):
setup.InitConfigObjects(cliConfPath, defaultDir)   // create objects, register all defaults
// ... set config file paths ...
setup.LoadDatadog(cfg, secretsComp, delegatedAuthComp, extraEnvVars)  // read file, decrypt, detect features
```

`InitConfigObjects` is safe to call multiple times (e.g., in tests) because it recreates the singletons each time.

In practice, agent binaries do not call these directly; they rely on `comp/core/config` (via `core.Bundle()`) to call `LoadDatadog` at the right point in the fx lifecycle.

### Real-world patterns

- **All agent binaries** import this package (via transitive deps from `comp/core/config`) to gain access to the initialized global config.
- **Process agent** (`cmd/process-agent/command/command.go`) imports `DefaultProcessAgentLogFile` to configure its logger before the config file is read.
- **Autodiscovery** (`cmd/agent/common/autodiscovery.go`) calls `setup.Datadog().GetBool("autoconf_config_files_poll")` to decide whether to poll config files.
- **Misconfig checks** (`cmd/agent/common/misconfig/`) combine `env.IsContainerized()` with `setup.Datadog().GetString("container_proc_root")` to find the right `/proc` path.
- **`pkg/config/env`** is called by `LoadDatadog` to run `DetectFeatures` after the YAML file is read, ensuring feature flags reflect both config-key overrides and environment variables.

## Pitfalls

- **Do not add keys to `config.go`.** The file `config.go` is for core startup logic; new keys belong in `common_settings.go` (shared with serverless) or a dedicated subsystem file.
- **`GetIPCAddress` may return an error.** Code that calls it must handle the error; the function validates that the address is local and rejects non-loopback addresses to prevent accidental remote exposure.
- **`Datadog()` returns `nil` before `init()` completes.** In tests that do not call `InitConfig`, the singleton is `nil`; use `pkg/config/mock` to get a properly initialised test config.
- **`LoadDatadog` is not idempotent.** Calling it multiple times on the same config object will merge the YAML file on top of itself and re-run feature detection. In tests, recreate the config object with `InitConfigObjects` or use `pkg/config/mock`.
