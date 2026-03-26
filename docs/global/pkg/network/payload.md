> **TL;DR:** `pkg/network/payload` provides lightweight routing-metadata types (`Via`, `Subnet`, `Interface`) attached to network flows, kept in a separate module to avoid pulling system-probe internals into packages that only need to inspect or forward routing information.

# pkg/network/payload

## Purpose

`pkg/network/payload` provides a small set of Go types that represent routing metadata attached to a network flow. These types are designed for JSON serialization (all fields carry `json:"..."` struct tags) and are kept in a separate module to avoid pulling system-probe-internal dependencies into packages that only need to inspect or forward routing information — for example, `pkg/networkpath`.

## Key elements

### Key interfaces

This package exposes no interfaces or functions beyond the struct types below.

### Key functions

All fields are accessed directly — no constructor functions or helper methods are provided.

### Key types

| Type | Fields | Description |
|---|---|---|
| `Via` | `Subnet Subnet`, `Interface Interface` | Routing decision for a flow — which subnet alias and network interface were used. Embedded in `network.ConnectionStats` in `pkg/network`. |
| `Subnet` | `Alias string` | Human-readable alias for a subnet (e.g. a VPC or CIDR label). |
| `Interface` | `HardwareAddr string` | MAC address of the egress network interface. |

All fields are tagged `omitempty` so that the JSON payload stays compact when routing information is unavailable.

### Configuration and build flags

This package carries no build constraints and no configuration. It is compiled on all platforms.

## Usage

`pkg/network/event_common.go` embeds `payload.Via` inside `network.ConnectionStats` to attach optional routing context to each tracked connection. The same `Via` struct is referenced by the marshal package when building the protobuf route index: `FormatConnection` calls `formatRouteIdx` which keys a deduplicated route table on `network.Via` values.

`pkg/networkpath/payload/pathevent.go` also imports this package to annotate path-discovery events with the same routing metadata schema.
