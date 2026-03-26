> **TL;DR:** `pkg/network/types` defines the minimal `ConnectionKey` four-tuple type used as a shared map key across both system-probe and process-agent code, avoiding import cycles by having no dependencies on system-probe internals.

# pkg/network/types

## Purpose

`pkg/network/types` defines the core network four-tuple type used as a map key throughout the network monitoring subsystem. It is intentionally minimal — a single struct with no dependencies on system-probe internals — so that it can be imported by both system-probe-side code (eBPF maps, USM protocol tracking) and process-agent-side code (encoding, USM index lookups) without creating an import cycle.

## Key elements

### Key interfaces

This package exposes no interfaces.

### Key types

| Symbol | Description |
|---|---|
| `ConnectionKey` | A compact representation of a TCP/UDP four-tuple: source IP (split into `SrcIPHigh`/`SrcIPLow` uint64 pair to hold IPv6 without allocation), destination IP (same split), `SrcPort uint16`, `DstPort uint16`. Fields are ordered for struct alignment — ports are grouped at the end. |
| `NewConnectionKey(saddr, daddr util.Address, sport, dport uint16) ConnectionKey` | Convenience constructor that converts `util.Address` values to the low/high representation via `util.ToLowHigh`. |
| `(ConnectionKey).String() string` | Human-readable `[src:sport <=> dst:dport]` format for logging and debugging. |

The IP representation uses two `uint64` fields (high + low) rather than `[16]byte` or `net.IP` to pack an IPv6 address into a value type that is hashable as a Go map key without heap allocation.

### Key functions

| Function | Description |
|---|---|
| `NewConnectionKey(saddr, daddr util.Address, sport, dport uint16) ConnectionKey` | Convenience constructor that converts `util.Address` values to the low/high representation via `util.ToLowHigh`. |
| `(ConnectionKey).String() string` | Human-readable `[src:sport <=> dst:dport]` format for logging and debugging. |

### Configuration and build flags

This package carries no build constraints and no configuration. It is compiled on all platforms to allow cross-binary sharing of the `ConnectionKey` type.

## Usage

`ConnectionKey` is used as the map key in `USMConnectionIndex` inside `pkg/network/encoding/marshal/usm.go`, which groups application-layer protocol statistics (HTTP, Kafka, etc.) by connection before encoding. It is also used in USM protocol tracking code across `pkg/network/protocols/` (HTTP, Redis, Postgres, Kafka) to key per-connection stat maps that are later read from eBPF maps.
