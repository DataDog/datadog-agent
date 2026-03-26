> **TL;DR:** A lightweight configuration helper that centralises the logic for deciding whether DogStatsD should start at all and whether the Core Agent or the Agent Data Plane is responsible for handling it.

# comp/dogstatsd/config

**Team:** agent-metric-pipelines

## Purpose

This package provides a lightweight configuration helper for DogStatsD. Its primary role is to centralize the logic that determines whether DogStatsD should be started at all, and whether the Core Agent or the Agent Data Plane (ADP) is responsible for handling it. This avoids scattered `config.GetBool("use_dogstatsd")` calls and makes the ADP hand-off logic easy to reason about in one place.

It is not an fx component in the traditional sense (no `Component` interface, no `Module()`). It is a plain struct instantiated directly by other components that need to make routing decisions about DogStatsD.

## Key elements

### Key types

`comp/dogstatsd/config/config.go`

```go
type Config struct {
    config config.Component
}

func NewConfig(config config.Component) *Config
```

### Key functions

| Method | Description |
|--------|-------------|
| `Enabled() bool` | Returns `true` when `use_dogstatsd` is `true`. This is the baseline check — if `false`, DogStatsD must not start in any form. |
| `EnabledInternal() bool` | Returns `true` when DogStatsD is enabled **and** the Agent Data Plane is not handling it. Only this method should gate Core Agent DogStatsD startup. |
| `enabledDataPlane() bool` (unexported) | Returns `true` when either the legacy `DD_ADP_ENABLED=true` env var is set, or both `data_plane.enabled` and `data_plane.dogstatsd.enabled` are `true`. |

### Configuration and build flags

| `use_dogstatsd` | ADP active | `Enabled()` | `EnabledInternal()` |
|-----------------|-----------|-------------|---------------------|
| false | any | false | false |
| true | false | true | true |
| true | true | true | false |

ADP detection uses two mechanisms (either is sufficient):

1. **Legacy env var:** `DD_ADP_ENABLED=true` — deprecated, still supported for backward compatibility.
2. **Config keys:** `data_plane.enabled` AND `data_plane.dogstatsd.enabled` both `true`.

| Key | Type | Description |
|-----|------|-------------|
| `use_dogstatsd` | bool | Master DogStatsD enable switch |
| `data_plane.enabled` | bool | Whether Agent Data Plane is running |
| `data_plane.dogstatsd.enabled` | bool | Whether ADP handles DogStatsD traffic |
| `DD_ADP_ENABLED` | env var | Legacy ADP signal (deprecated) |

## Usage

`NewConfig` is called by consuming components; it is not auto-wired by fx. Current call sites:

- `comp/dogstatsd/status/statusimpl` — decides whether to register the DogStatsD status provider. If `EnabledInternal()` is `false`, no status section is added for DogStatsD.
- Any future component needing to distinguish "DogStatsD disabled entirely" from "DogStatsD handed off to ADP" should use this helper rather than reading config keys directly.

> Note: The package comment calls out that listener addresses and other shared DogStatsD settings should be ported here over time. If you find yourself adding a new setting used by multiple `comp/dogstatsd` packages, consider adding a method to `Config` rather than reading the config key directly in each package.
