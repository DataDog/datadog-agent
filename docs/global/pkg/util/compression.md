> **TL;DR:** Defines a `Compressor` interface with interchangeable backends (zlib, zstd, gzip, noop) so the serializer and forwarder can compress payloads via a single API, with the concrete algorithm chosen by build tags and runtime configuration.

# pkg/util/compression

## Purpose

`pkg/util/compression` provides a single `Compressor` interface and a set of interchangeable
backend implementations for compressing agent payloads before they are forwarded to Datadog
intake endpoints. The abstraction lets the serializer and forwarder be written against one
interface while the concrete algorithm (zlib, zstd, gzip, or none) is chosen at build time
and/or runtime configuration.

## Key elements

### Key interfaces

#### `Compressor` interface

```go
type Compressor interface {
    Compress(src []byte) ([]byte, error)
    Decompress(src []byte) ([]byte, error)
    CompressBound(sourceLen int) int
    ContentEncoding() string
    NewStreamCompressor(output *bytes.Buffer) StreamCompressor
}
```

- `Compress` / `Decompress` — one-shot byte-slice compression.
- `CompressBound(sourceLen)` — returns the maximum possible size of a compressed output for a given input length. Used to pre-allocate buffers before compressing.
- `ContentEncoding()` — returns the HTTP `Content-Encoding` header value to attach to forwarded payloads (e.g. `"deflate"`, `"zstd"`, `"gzip"`).
- `NewStreamCompressor(output)` — returns a `StreamCompressor` that writes compressed data incrementally into `output`; used for streaming large payloads.

#### `StreamCompressor` interface

```go
type StreamCompressor interface {
    io.WriteCloser
    Flush() error
}
```

Wraps a compressor writer so the serializer can stream chunks into it. The caller writes
chunks, calls `Flush()` at boundaries, and `Close()` when done.

### Configuration and build flags

#### Kind constants and encoding constants

| Constant | Value | Meaning |
|---|---|---|
| `ZlibKind` | `"zlib"` | Select zlib (deflate) compression |
| `ZstdKind` | `"zstd"` | Select Zstandard compression |
| `GzipKind` | `"gzip"` | Select gzip compression |
| `NoneKind` | `"none"` | No compression |
| `ZlibEncoding` | `"deflate"` | HTTP Content-Encoding for zlib |
| `ZstdEncoding` | `"zstd"` | HTTP Content-Encoding for zstd |
| `GzipEncoding` | `"gzip"` | HTTP Content-Encoding for gzip |

### Key types

#### `ZstdCompressionLevel`

```go
type ZstdCompressionLevel int
```

A typed wrapper around `int` for the zstd compression level, used in `Requires` structs of
zstd backends to keep the API self-documenting.

#### Backend implementations

Each sub-package exposes a `New(Requires) compression.Compressor` constructor. The
`Requires` struct carries backend-specific configuration (compression level).

| Sub-package | Algorithm | Build tag required | Notes |
|---|---|---|---|
| `impl-gzip` | gzip (stdlib) | none | Level configurable; defaults to 6 in selector |
| `impl-zlib` | zlib/deflate (stdlib) | `zlib` | No level option (fixed default) |
| `impl-zstd` | Zstandard via `github.com/DataDog/zstd` (CGO) | `zstd` | Level configurable |
| `impl-zstd-nocgo` | Zstandard via `github.com/klauspost/compress/zstd` (pure Go) | none | Not wired into the selector; selected explicitly via the fx component system. Concurrency and window size tunable via `ZSTD_NOCGO_CONCURRENCY` and `ZSTD_NOCGO_WINDOW` env vars |
| `impl-noop` | No compression | none | Trivial pass-through |

### Key functions

#### `selector` sub-package

`selector.NewCompressor(kind string, level int) compression.Compressor` is the main factory
used by the serializer. It dispatches on the `kind` string and respects what is compiled in
via build tags:

| Build tags | Available algorithms |
|---|---|
| `zlib && zstd` | zlib, zstd (CGO), gzip, none |
| `zlib && !zstd` | zlib, gzip, none (requesting zstd falls back to zlib with a warning) |
| `!zlib && !zstd` | gzip, none |

`selector.FromConfig(cfg config.Reader) compression.Compressor` reads the
`serializer_compressor_kind` config key (and `serializer_zstd_compressor_level` for zstd)
and returns the appropriate compressor. This is the entry point for runtime configuration.

`selector.NewNoopCompressor()` is a convenience function returning a noop compressor,
useful when a compressor placeholder is needed before configuration is resolved.

## Usage

### Serializer and stream compressor

`pkg/serializer/internal/stream` uses `Compressor.NewStreamCompressor` to compress metrics
and sketch payloads as they are being built, avoiding a second in-memory copy.
`Compressor.CompressBound` is called to pre-size the target buffer.

`pkg/serializer/internal/metrics` wraps a `Compressor` for series and sketch payloads
before they reach the forwarder.

### Fx component wiring

`comp/serializer/metricscompression` provides an fx component that wraps a `Compressor`.
The concrete implementation is injected at startup via the selector. Callers depending on
compression import `comp/serializer/metricscompression/def` (the interface) and receive the
concrete compressor through dependency injection.

### Logs and event-platform forwarder

`pkg/logs/pipeline/pipeline.go` and `comp/forwarder/eventplatform/eventplatformimpl/` hold
a `Compressor` reference acquired from the fx graph to compress log and event payloads.

### OTEL exporter

`comp/otelcol/otlp/components/exporter/serializerexporter` uses the same `Compressor`
interface to compress OTLP-serialized metrics before forwarding.

### Choosing a backend at build time

The standard agent binary is built with `zlib` and `zstd` tags, making both algorithms
available. Minimal builds (e.g. IoT agent) may omit these tags to reduce binary size, in
which case only gzip and noop are available. The `impl-zstd-nocgo` backend is used in
environments where CGO is disabled; it must be wired in explicitly through the fx system
rather than via the selector.

---

## Cross-references

| Topic | Document |
|---|---|
| `comp/serializer/metricscompression` — fx component wrapping a single shared `Compressor` for the metrics pipeline; reads config via `selector.FromConfig` | [`comp/serializer/metricscompression`](../../comp/serializer/metricscompression.md) |
| `comp/serializer/logscompression` — factory-style component that calls `selector.NewCompressor(kind, level)` once per log endpoint | [`comp/serializer/logscompression`](../../comp/serializer/logscompression.md) |
| `pkg/serializer` — consumes `Compressor` via fx injection; uses `NewStreamCompressor` and `CompressBound` in the streaming JSON builder | [`pkg/serializer`](../serializer.md) |

### Component vs. package: which to import

| Situation | What to use |
|---|---|
| Adding compression to a new fx-wired component in the metrics pipeline | Inject `metricscompression.Component` (alias for `compression.Compressor`) via fx |
| Compressing log payloads per-endpoint in the logs pipeline | Inject `logscompression.Component` and call `NewCompressor(kind, level)` |
| Writing a test that needs a no-op compressor without fx | Call `selector.NewNoopCompressor()` directly |
| Implementing a new compression backend | Create a sub-package under `pkg/util/compression/impl-<name>/` with a `New(Requires) compression.Compressor` constructor and wire it into `selector` |

### Data flow through the metrics serialization path

```
comp/serializer/metricscompression (fx)
  └─ selector.FromConfig(cfg)       ← reads serializer_compressor_kind
       └─ returns compression.Compressor
            ├─ pkg/serializer.Serializer.Compress(payload)
            │     sets Content-Encoding header via ContentEncoding()
            └─ internal/stream.JSONPayloadBuilder
                  NewStreamCompressor(buf) → streams compressed chunks
                  CompressBound(n)         → pre-allocates output buffer
```
