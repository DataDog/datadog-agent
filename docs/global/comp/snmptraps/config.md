> **TL;DR:** `comp/snmptraps/config` parses and validates the SNMP traps listener configuration from `datadog.yaml`, derives the authoritative engine ID from the hostname, and exposes the resulting `TrapsConfig` struct to all other traps sub-components via a single `Get()` call.

# comp/snmptraps/config

**Team:** network-device-monitoring-core

## Purpose

This component parses and provides the SNMP traps listener configuration to the rest of the `comp/snmptraps` subsystem. It reads the `network_devices.snmp_traps` config block, applies defaults, derives a stable authoritative engine ID from the agent hostname, and makes the resulting `TrapsConfig` struct available to other components (listener, server, forwarder) through a single `Get()` call.

## Key elements

### Key interfaces

`comp/snmptraps/config/component.go`

```go
type Component interface {
    Get() *TrapsConfig
}
```

`Get()` returns the fully-validated `TrapsConfig` parsed at startup. The struct is not reloaded at runtime; configuration changes require an agent restart.

### Key types

#### `TrapsConfig`

`comp/snmptraps/config/config.go`

```go
type TrapsConfig struct {
    Enabled          bool
    Port             uint16      // default 9162
    Users            []UserV3
    CommunityStrings []string
    BindHost         string      // default "0.0.0.0"
    StopTimeout      int         // seconds, default 5
    Namespace        string
    authoritativeEngineID string  // unexported, derived from hostname
}
```

#### `UserV3`

SNMPv3 user credentials used to build the USM security parameters table:

```go
type UserV3 struct {
    Username     string  // "user" key (preferred)
    UsernameLegacy string // "username" key (backward compat)
    AuthKey      string
    AuthProtocol string  // e.g. "MD5", "SHA", "SHA224", ...
    PrivKey      string
    PrivProtocol string  // e.g. "DES", "AES", "AES192", ...
}
```

### Key functions

| Method | Description |
|--------|-------------|
| `SetDefaults(host, namespace string) error` | Fills in default values and computes the authoritative engine ID (FNV-128 of the hostname, prefixed with SNMP OID bytes). Returns an error if the namespace is invalid. |
| `Addr() string` | Returns `"<BindHost>:<Port>"` — the UDP address the listener binds to. |
| `BuildSNMPParams(logger) (*gosnmp.GoSNMP, error)` | Constructs a `gosnmp.GoSNMP` struct ready to be used by the trap listener. Uses SNMPv2c when no users are configured; SNMPv3 with a USM security parameters table otherwise. |
| `GetPacketChannelSize() int` | Returns the fixed channel buffer size (100) for the packets channel between listener and server. |

**`IsEnabled` helper:**

```go
func IsEnabled(conf config.Component) bool
```

A package-level convenience function that reads `network_devices.snmp_traps.enabled` directly, without constructing a full `TrapsConfig`. Used for fast early-exit checks.

**Authoritative engine ID** — computed once during `SetDefaults` as:

```
[0x80, 0xff, 0xff, 0xff, 0xff] + FNV-128(hostname)
```

The first byte (`0x80`) and the next four bytes are SNMP-mandated framing. The remaining 16 bytes ensure the engine ID is unique per agent instance.

### Configuration and build flags

All keys live under `network_devices.snmp_traps`:

| Key | Default | Description |
|-----|---------|-------------|
| `enabled` | `false` | Enable/disable the trap listener |
| `port` | `9162` | UDP port to listen on |
| `bind_host` | `0.0.0.0` | Interface to bind |
| `stop_timeout` | `5` | Seconds to wait for graceful shutdown |
| `community_strings` | `[]` | SNMPv1/v2c community strings |
| `users` | `[]` | SNMPv3 user definitions |
| `namespace` | (global `network_devices.namespace`) | Namespace tag for trap events |

## Usage

### Wire-up

The production module is `configimpl.Module()` (`comp/snmptraps/config/configimpl/service.go`). It:

1. Calls `hostname.Component.Get()` to resolve the agent hostname.
2. Passes the hostname and `config.Component` to `trapsconf.ReadConfig()`.
3. Registers the resulting `*TrapsConfig` as the `Component` implementation.

Dependencies: `comp/core/config`, `comp/core/hostname`.

### Test module

`configimpl.MockModule()` provides a default `TrapsConfig{Enabled: true}` and allows tests to override it with `fx.Replace(&trapsconf.TrapsConfig{...})`. Fields not explicitly set receive defaults through `SetDefaults`.

### Consumers

- `comp/snmptraps/listener` — calls `Get()` to obtain the bind address, port, and `BuildSNMPParams()` output.
- `comp/snmptraps/server` — uses `Get()` for the stop timeout and packet channel size.
- `comp/snmptraps/forwarder` — uses `Get()` for the namespace tag applied to forwarded trap events.
- `pkg/config/basic` — calls `IsEnabled()` during basic config validation.

