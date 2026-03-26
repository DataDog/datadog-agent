> **TL;DR:** `pkg/security/config` is the single source of truth for all CWS configuration, defining and loading every `runtime_security_config.*` key from `system-probe.yaml` into strongly-typed `Config` and `RuntimeSecurityConfig` structs.

# pkg/security/config

## Purpose

Defines and loads the complete CWS (Cloud Workload Security) configuration. It is the single source of truth for every tunable in `runtime_security_config.*` from `system-probe.yaml`. It also defines supporting types used when requesting or storing activity dumps.

## Key elements

### Key types

#### `Config`

Top-level config container:

```go
type Config struct {
    Probe           *pconfig.Config          // system-probe / eBPF probe settings
    RuntimeSecurity *RuntimeSecurityConfig   // CWS-specific settings
}
```

Created by `NewConfig()` which calls both `pconfig.NewConfig()` and `NewRuntimeSecurityConfig()`.

#### `RuntimeSecurityConfig`

Large struct (450+ lines) mapping every `runtime_security_config.*` key. Broad categories:

| Category | Representative fields |
|----------|-----------------------|
| Core feature flags | `RuntimeEnabled`, `FIMEnabled`, `EBPFLessEnabled` |
| Communication | `SocketPath`, `CmdSocketPath`, `EventServerBurst`, `EventServerRate`, `EventServerRetention` |
| Self-test | `SelfTestEnabled`, `SelfTestSendReport` |
| Remote config | `RemoteConfigurationEnabled`, `RemoteConfigurationDumpPolicies` |
| Activity dump | `ActivityDumpEnabled`, `ActivityDumpTracedCgroupsCount`, `ActivityDumpCgroupDumpTimeout`, `ActivityDumpMaxDumpSize` (func), `ActivityDumpLocalStorageFormats`, etc. |
| Event sampling | `EventSamplingOpenEnabled/Rate`, `EventSamplingDNSEnabled/Rate`, etc. |
| Security profiles | `SecurityProfileEnabled`, `SecurityProfileV2Enabled`, `SecurityProfileMaxCount`, `SecurityProfileNodeEvictionTimeout` |
| Anomaly detection | `AnomalyDetectionEnabled`, `AnomalyDetectionDefaultMinimumStablePeriod`, `AnomalyDetectionRateLimiterNumEventsAllowed` |
| SBOM resolver | `SBOMResolverEnabled`, `SBOMResolverWorkloadsCacheSize` |
| Hash resolver | `HashResolverEnabled`, `HashResolverMaxFileSize`, `HashResolverHashAlgorithms` |
| Enforcement | `EnforcementEnabled`, `EnforcementRawSyscallEnabled`, enforcement disarmer settings |
| Windows | `WindowsFilenameCacheSize`, `WindowsRegistryCacheSize`, `ETWEventsChannelSize`, `ETWEventsMaxBuffers` |
| Misc | `UserSessionsCacheSize`, `IMDSIPv4`, `SysCtlEnabled`, `FileMetadataResolverEnabled` |

#### `Policy`

```go
type Policy struct {
    Name  string
    Files []string
    Tags  []string
}
```

Represents a policy-file entry in configuration (name, globs for files, tags).

#### `StorageRequest` / `StorageFormat` / `StorageType` (`dump.go`)

Used when requesting activity-dump persistence. `StorageFormat` and `StorageType` are stringer-generated enums (`//go:generate go run golang.org/x/tools/cmd/stringer`).

### Key functions

| Function | Description |
|----------|-------------|
| `NewConfig()` | Creates a combined `Config` from both probe and CWS configs. |
| `NewRuntimeSecurityConfig()` | Reads all `runtime_security_config.*` keys from `pkgconfigsetup.SystemProbe()` and returns a populated `RuntimeSecurityConfig`. |

### Configuration and build flags

#### Platform-specific sanitization (`config_linux.go` / `config_others.go`)

`sanitizePlatform()` is called after construction. On Linux with eBPFLess mode, it forces `ActivityDumpEnabled = false` and `SecurityProfileEnabled = false` since those features require full eBPF.

#### Constants

`ADMinMaxDumSize = 100` — minimum value enforced for `activity_dump.max_dump_size`.

`defaultKernelCompilationFlags` — list of ~80 kernel `CONFIG_*` options that CWS queries for the sysctl snapshot feature.

## Usage

`NewConfig()` is called from `pkg/security/probe` to build the probe, and from `cmd/security-agent` to configure the security agent. Most subsystems receive a `*RuntimeSecurityConfig` rather than the full `Config`.

Example:

```go
cfg, err := config.NewConfig()
if err != nil { ... }
if cfg.RuntimeSecurity.ActivityDumpEnabled {
    // set up activity dump manager
}
```

`StorageRequest` is used by the activity-dump manager and the gRPC API layer when callers request a specific dump format or storage type.
