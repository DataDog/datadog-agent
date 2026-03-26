# pkg/util/otel

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/otel`

## Purpose

`pkg/util/otel` provides a thin wrapper around the `attributes.GatewayUsage` type from the OpenTelemetry mapping library. Its goal is to track whether the Datadog OTLP pipeline is running in **gateway mode** — a deployment topology where the OTel Collector forwards data through a Datadog gateway instead of sending it directly. The package solves two concrete problems:

1. **Nil-interface safety.** A `nil *attributes.GatewayUsage` and a `nil attributes.HostFromAttributesHandler` interface are distinct in Go (the interface also carries type information). Returning the concrete type directly where an interface is expected causes a non-nil interface wrapping a nil pointer, which can trigger nil-pointer panics. The wrapper's `GetHostFromAttributesHandler` method enforces the safe conversion.

2. **Dual-signal gateway detection.** Gateway mode can be indicated either by the environment variable `DD_OTELCOLLECTOR_GATEWAY_MODE` (set by the Helm chart or the Datadog Operator) or by usage telemetry collected at runtime from OTLP attributes. The wrapper merges both signals, giving the env variable priority.

## Key Elements

### `GatewayUsage` (struct)

The central type. Holds a pointer to `attributes.GatewayUsage` (for runtime attribute-based detection) and an `*atomic.Bool` (for the env-variable signal). Zero value represents a disabled/no-op state.

### `NewGatewayUsage(gatewayModeSet bool) GatewayUsage`

Creates an active `GatewayUsage`. Pass `true` if `DD_OTELCOLLECTOR_GATEWAY_MODE` is set, `false` otherwise. Both the attribute-based and the env-var-based trackers are initialized.

### `NewDisabledGatewayUsage() GatewayUsage`

Creates an inert `GatewayUsage` with nil internals. Use this when the OTLP pipeline is not running in a gateway-capable context.

### `(*GatewayUsage).GetHostFromAttributesHandler() attributes.HostFromAttributesHandler`

Returns the underlying `*attributes.GatewayUsage` as the `HostFromAttributesHandler` interface, or a true `nil` interface when the struct was disabled. This is the safe accessor that callers should use instead of casting the pointer directly.

### `(*GatewayUsage).EnvVarValue() float64`

Returns `1.0` if the gateway-mode env var is set, `0.0` otherwise. Used for reporting the env-var state as a gauge metric.

### `(*GatewayUsage).Gauge() (float64, bool)`

Returns the combined gateway usage gauge and whether gateway mode is active. Priority: env var > attribute-based detection. Returns `(0, false)` when the struct is disabled.

## Usage

The OTel serializer exporter (`comp/otelcol/otlp/components/exporter/serializerexporter/`) is the primary consumer:

```go
import "github.com/DataDog/datadog-agent/pkg/util/otel"

// At factory/constructor time:
gwMode := os.Getenv("DD_OTELCOLLECTOR_GATEWAY_MODE") != ""
gatewayUsage := otel.NewGatewayUsage(gwMode)

// When constructing per-request consumers:
consumer := newConsumer(params, gatewayUsage)

// Inside the consumer — safe interface extraction:
handler := gatewayUsage.GetHostFromAttributesHandler()
// handler is a true nil interface if disabled, safe to pass to attribute resolvers.

// For telemetry reporting:
if val, ok := gatewayUsage.Gauge(); ok {
    sender.Gauge("dd.otelcol.gateway_mode", val, "", nil)
}
```

Importers include:
- `comp/otelcol/otlp/components/exporter/serializerexporter/` — serializer exporter factory and consumer
- `comp/otelcol/otlp/components/exporter/logsagentexporter/` — logs exporter
- `comp/otelcol/otlp/components/exporter/datadogexporter/` — Datadog exporter
- `comp/otelcol/collector/impl/` — OTel collector component
