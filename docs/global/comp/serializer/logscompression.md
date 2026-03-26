> **TL;DR:** Provides a per-endpoint compressor factory for the logs pipeline, allowing each HTTP destination to independently select a compression algorithm and level at pipeline setup time.

# comp/serializer/logscompression

**Team:** agent-log-pipelines

## Purpose

`comp/serializer/logscompression` provides a factory for creating compressors used when serializing log payloads for transmission to the Datadog intake. Unlike the metrics compressor (which reads the compression kind from configuration at startup and returns a single fixed instance), the logs compressor factory is called once per endpoint at pipeline setup time, receiving the desired algorithm and compression level as explicit parameters. This allows different endpoints to use different compression settings without shared state.

## Key Elements

### Key interfaces

#### Interface (`comp/serializer/logscompression/def/component.go`)

```go
type Component interface {
    NewCompressor(kind string, level int) compression.Compressor
}
```

`kind` is a string identifying the compression algorithm (e.g., `"zstd"`, `"gzip"`, `"none"`). `level` is algorithm-specific. The method delegates to `pkg/util/compression/selector.NewCompressor`, which resolves the algorithm by name and returns a `compression.Compressor` ready to compress individual payloads.

### Key types

#### Implementation (`impl/logscompressionimpl.go`)

The production implementation is a zero-field struct. `NewCompressor` is stateless — it creates a new `compression.Compressor` on every call.

### Configuration and build flags

#### fx modules

| Package | Description |
|---|---|
| `comp/serializer/logscompression/fx` | Production module — provides the real implementation |
| `comp/serializer/logscompression/fx-mock` | Test module (`//go:build test`) — `NewCompressor` always returns a no-op compressor (`selector.NewNoopCompressor()`), avoiding compression overhead in unit tests |

## Usage

### Wiring in production

```go
// Add to the fx app:
logscompressionfx.Module()  // from comp/serializer/logscompression/fx
```

The `comp/logs/agent` implementation injects `logscompression.Component` and passes it to `pipeline.NewProvider`, which in turn passes it to each `pipeline.Pipeline` and ultimately to the HTTP sender where `NewCompressor` is called with the endpoint's configured kind and level.

### How it is called in the pipeline

```go
// pkg/logs/pipeline/pipeline.go (simplified)
compression logscompression.Component

// per-pipeline setup:
compressor := compression.NewCompressor(endpoint.CompressionKind, endpoint.CompressionLevel)
```

Each pipeline instance receives its own independent compressor, so compression configuration can differ between main and additional endpoints (e.g., a Vector proxy endpoint might use no compression while the main intake uses zstd).

### Testing

Use `fx-mock.MockModule()` to inject the no-op compressor in test fx apps:

```go
fx.Options(
    logscompressionfxmock.MockModule(),
    // ...
)
```

Or use `fx-mock.NewMockCompressor()` directly to get an instance without fx.

### Distinction from metrics compression

The metrics serializer has a separate `comp/serializer/compression` component. That component reads the compression algorithm from configuration once at wire time and returns a pre-configured instance. The logs compressor takes parameters at call time because log endpoints can each have independent compression settings and are potentially rebuilt during transport upgrades (TCP → HTTP) without restarting the entire agent.

## Related documentation

| Document | Relationship |
|---|---|
| [pkg/util/compression.md](../../pkg/util/compression.md) | Defines the `Compressor` and `StreamCompressor` interfaces; `NewCompressor` delegates to `selector.NewCompressor` in this package; the `kind`/`level` constants (`ZstdKind`, `GzipKind`, etc.) and build-tag rules are documented there |
| [pkg/logs/pipeline.md](../../pkg/logs/pipeline.md) | `pipeline.NewProvider` receives this component and calls `NewCompressor(endpoint.CompressionKind, endpoint.CompressionLevel)` once per `Pipeline` instance; describes how compression interacts with `BatchStrategy` and the HTTP transport |
| [comp/logs/agent.md](../logs/agent.md) | Injects this component as a dependency and passes it to `pipeline.NewProvider`; also rebuilds the pipeline (and thus recreates compressors) on TCP → HTTP transport upgrades |
| [comp/serializer/metricscompression.md](metricscompression.md) | The metrics-pipeline counterpart: configuration-driven, single shared instance; contrasted in the "Distinction from metrics compression" section above |
