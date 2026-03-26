# comp/serializer/metricscompression — Metrics Compression Component

**Import path:** `github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def`
**Team:** agent-metric-pipelines
**Importers:** ~29 packages

## Purpose

`comp/serializer/metricscompression` provides a single, injectable `Compressor` instance for use by the metrics serialization pipeline. Rather than each consumer independently selecting and instantiating a compression algorithm, this component reads the agent configuration once at startup, constructs the right compressor, and makes it available via fx dependency injection.

Consumers (the serializer, stream compressor, check samplers, etc.) receive the component through normal fx injection and call the `Compressor` interface methods. They remain unaware of whether the actual algorithm is zlib, zstd, gzip, or a no-op; the component abstracts that entirely.

## Package layout

| Package | Role |
|---|---|
| `comp/serializer/metricscompression/def` | `Component` interface (aliases `compression.Compressor`) |
| `impl/` | Constructor `NewCompressorReq` (config-driven) and `NewCompressorReqOtel` (zlib-only for OTel) |
| `fx/` | Standard `Module()` wiring via `NewCompressorReq` |
| `fx-otel/` | OTel-specific `Module()` wiring via `NewCompressorReqOtel` |
| `fx-mock/` | `MockModule()` for tests — provides a no-op (`NoneKind`) compressor |

## Component interface

`Component` is a direct alias for `pkg/util/compression.Compressor`:

```go
type Compressor interface {
    Compress(src []byte) ([]byte, error)
    Decompress(src []byte) ([]byte, error)
    CompressBound(sourceLen int) int
    ContentEncoding() string
    NewStreamCompressor(output *bytes.Buffer) StreamCompressor
}
```

`ContentEncoding()` returns the HTTP `Content-Encoding` header value (`"deflate"` for zlib, `"zstd"` for zstd, `"gzip"` for gzip, or `""` for no-op). The serializer uses this to set the correct HTTP header before forwarding payloads.

`NewStreamCompressor` returns a `StreamCompressor` (`io.WriteCloser` + `Flush()`) for streaming compression of large payloads built incrementally by `pkg/serializer/internal/stream`.

## Configuration

The standard `fx/` module reads two config keys:

| Key | Effect |
|---|---|
| `serializer_compressor_kind` | Algorithm: `"zlib"` (default), `"zstd"`, `"gzip"`, or `"none"` |
| `serializer_zstd_compressor_level` | Compression level when `zstd` is selected |

The OTel module (`fx-otel/`) always uses zlib and ignores configuration.

## fx wiring

```go
// Standard agent startup (reads from config):
metricscompressionfx.Module()

// OTel collector pipeline (always zlib):
metricscompressionfxotel.Module()

// Tests (no-op, no config required):
metricscompressionfxmock.MockModule()
```

### Dependencies injected by fx

`NewCompressorReq` requires `config.Component` to read `serializer_compressor_kind`. `NewCompressorReqOtel` and the mock constructor take no dependencies.

## Usage patterns

**Receiving via fx injection (most common):**

```go
type deps struct {
    fx.In
    Compressor metricscompression.Component
}

func (s *mySerializer) compress(data []byte) ([]byte, error) {
    return s.Compressor.Compress(data)
}
```

**Building a streaming payload with `NewStreamCompressor`:**

```go
buf := new(bytes.Buffer)
sc := compressor.NewStreamCompressor(buf)
sc.Write(chunk)
sc.Flush()
sc.Close()
// buf now contains the compressed stream
```

## Key consumers

- `pkg/serializer/internal/stream.Compressor` — wraps the component as its compression strategy when building chunked JSON or protobuf payloads
- `pkg/serializer.Serializer` — sets `Content-Encoding` HTTP headers based on `ContentEncoding()` and passes the compressor to the stream builder
- `pkg/aggregator.AgentDemultiplexer` — forwards the component to the serializer during demultiplexer construction
- `comp/aggregator/demultiplexer/demultiplexerimpl` — wires the component into the demultiplexer fx graph
- `comp/otelcol/otlp/components/exporter/serializerexporter` — uses the OTel module variant for the OTLP exporter

## Related documentation

| Document | Relationship |
|---|---|
| [pkg/serializer.md](../../pkg/serializer.md) | The `Serializer` struct receives `compression.Compressor` (this component's type) at construction; `split.CheckSizeAndSerialize` and `internal/stream.JSONPayloadBuilder` both consume it; `ContentEncoding()` drives the `Content-Encoding` HTTP header |
| [pkg/util/compression.md](../../pkg/util/compression.md) | Defines the `Compressor` and `StreamCompressor` interfaces that `Component` aliases; documents all backend implementations (zlib, zstd, gzip, noop) and the `selector.FromConfig` entry point used by the standard `fx/` module |
| [comp/serializer/logscompression.md](logscompression.md) | The logs-pipeline counterpart: factory-style (parameters at call time, one compressor per endpoint) rather than a single shared instance; both delegate to `pkg/util/compression/selector` |
| [comp/aggregator/demultiplexer.md](../aggregator/demultiplexer.md) | Injects this component (`compression.Component`) and forwards it to `AgentDemultiplexer`, which passes it to the serializer; the demultiplexer is the primary production consumer of this component in the metrics path |
