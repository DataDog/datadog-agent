> **TL;DR:** Uniform API for querying FIPS 140-2 compliance status at runtime — selects among four build-tag-gated implementations (standard, BoringCrypto, Microsoft Go Linux, Microsoft Go Windows) and governs whether the FIPS proxy is activated.

# pkg/fips

## Purpose

`pkg/fips` provides a uniform API for querying whether the agent binary was compiled and is
running in FIPS 140-2 compliant mode. Because FIPS support is entirely a build-time concern, the
package uses build tags to select among four implementations. Callers never need to know which
cryptographic backend is in use.

The package also governs the interaction between the native FIPS agent flavor (`datadog-fips-agent`)
and the FIPS proxy (`fips.enabled` config option): when the native FIPS flavor is active, the
proxy is intentionally disabled.

## Key elements

### Key functions

The package exposes two functions with identical signatures across all build variants:

**`Enabled() (bool, error)`**

Returns `(true, nil)` if the running agent is operating in FIPS mode. Returns `(false, nil)` for
standard builds. May return a non-nil error on Windows (registry read failure). Callers should
treat an error as "FIPS status unknown", not "FIPS disabled".

**`Status() string`**

Returns a human-readable string suitable for display in `agent status` output:

| Value | Meaning |
|---|---|
| `"enabled"` | FIPS mode is active |
| `"disabled"` | Built with a FIPS-capable backend but FIPS mode is not active |
| `"not available"` | Standard build without any FIPS backend |

The status page adds a fourth value, `"proxy"`, when `Status()` returns `"not available"` but
`fips.enabled` is set in the config (indicating the FIPS proxy is handling compliance).

### Configuration and build flags

The implementation file selected at compile time depends on build tags:

| File | Build constraint | Behavior |
|---|---|---|
| `fips_disabled.go` | `!goexperiment.boringcrypto && !goexperiment.systemcrypto` | Standard build. `Enabled()` always returns `(false, nil)`. |
| `fips_goboring.go` | `goexperiment.boringcrypto` | Go BoringCrypto backend. Imports `crypto/tls/fipsonly` (forces TLS to FIPS-approved suites). `Enabled()` always returns `(true, nil)`. |
| `fips_msftgo.go` | `goexperiment.systemcrypto && !windows && !goexperiment.boringcrypto` | Microsoft Go on Linux. Binary is built with `requirefips`; panics at startup if OpenSSL is not installed in FIPS mode. `Enabled()` always returns `(true, nil)`. |
| `fips_msftgo_windows.go` | `goexperiment.systemcrypto && windows && !goexperiment.boringcrypto` | Microsoft Go on Windows. Queries `HKLM\SYSTEM\CurrentControlSet\Control\Lsa\FipsAlgorithmPolicy\Enabled` (DWORD). Returns the registry value as the enabled state. |

## Usage

### Config setup — disabling the FIPS proxy for native FIPS builds

```go
// pkg/config/setup/config.go
fipsFlavor, err := pkgfips.Enabled()
if err != nil { return err }
if fipsFlavor {
    log.Debug("FIPS mode is enabled in the agent. Ignoring fips-proxy settings")
    return nil
}
if !config.GetBool("fips.enabled") { ... }
```

When `Enabled()` returns `true`, the config setup skips the proxy endpoint configuration
entirely. This prevents the native FIPS agent from accidentally routing traffic through the
(non-FIPS) proxy.

### Status page header

```go
// comp/core/status/statusimpl/common_header_provider.go
func populateFIPSStatus(config config.Component) string {
    fipsStatus := fips.Status()
    if fipsStatus == "not available" && config.GetString("fips.enabled") == "true" {
        return "proxy"
    }
    return fipsStatus
}
```

### Inventory agent metadata

```go
// comp/metadata/inventoryagent/inventoryagentimpl/inventoryagent.go
if val, err := fips.Enabled(); err == nil {
    metadata["feature_fips_enabled"] = val
}
```

### Python runtime initialization

```go
// pkg/collector/python/init.go
fipsEnabled, err := fips.Enabled()
if err == nil && fipsEnabled {
    // restrict Python cryptography to FIPS-approved algorithms
}
```

### config/utils miscellaneous

```go
// pkg/config/utils/miscellaneous.go
isFipsAgent, _ := pkgfips.Enabled()
conf["fips_proxy_enabled"] = strconv.FormatBool(config.GetBool("fips.enabled") && !isFipsAgent)
```

This ensures the `fips_proxy_enabled` config field is always `false` when the native FIPS agent
flavor is in use.

## Notes

- The package is its own Go module (`github.com/DataDog/datadog-agent/pkg/fips`), which means it
  can be imported with FIPS-specific build tags without pulling the rest of the agent module
  graph.
- The `goexperiment.boringcrypto` and `goexperiment.systemcrypto` tags are set by the build
  system, not by passing `-tags` manually. Use the dedicated `datadog-fips-agent` build flavor
  to produce a compliant binary.
- On BoringCrypto builds, importing `crypto/tls/fipsonly` is a side-effect import that restricts
  the TLS stack at program init; there is no runtime toggle.
- On the Microsoft Go Linux build, the FIPS requirement is enforced by the linker (`requirefips`
  flag); the agent binary will not start if the OS OpenSSL library is not in FIPS mode. This is
  the strictest enforcement mode and cannot be overridden at runtime.

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `comp/core/status` | [status.md](../../comp/core/status.md) | The `statusimpl/common_header_provider.go` header provider calls `fips.Status()` to populate the FIPS line in the agent status page. It maps `"not available"` → `"proxy"` when `fips.enabled` is set in config. To add a new status surface, implement `status.HeaderProvider` and register it via the `"header_status"` fx value group. |
| `pkg/config` | [config.md](../config/config.md) | `pkg/config/setup/config.go` calls `fips.Enabled()` during `LoadDatadog` to decide whether to skip FIPS-proxy endpoint configuration entirely. The `fips.enabled` config key is the toggle that activates the proxy path when the native FIPS flavor is absent. |
| `pkg/util/log` | [log.md](../util/log.md) | `pkg/config/setup` logs a `Debug` message when `Enabled()` returns `true` (proxy settings ignored). Callers should use `pkglog.Debugf` / `pkglog.Warnf` from `pkg/util/log` rather than `fmt.Print` when reporting FIPS state changes. |
