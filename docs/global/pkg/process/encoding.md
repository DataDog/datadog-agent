# pkg/process/encoding

> **TL;DR:** `pkg/process/encoding` provides content-negotiated (protobuf/JSON) serialization and deserialization of per-process stats exchanged between system-probe and the process-agent over a Unix socket.

**Import path:** `github.com/DataDog/datadog-agent/pkg/process/encoding`

## Purpose

Provides content-negotiated serialization and deserialization of per-process stats (`ProcStatsWithPerm`) exchanged between system-probe and the process-agent over a Unix socket. Callers select a format based on the HTTP `Accept` / `Content-Type` header; the package supports Protobuf (preferred) and JSON (fallback).

A sub-package `pkg/process/encoding/request` provides the same content-negotiation pattern for the inbound `ProcessStatRequest` message (process-agent → system-probe).

## Key Elements

### Key interfaces

#### Interfaces (`encoding.go`)

| Interface | Method(s) | Description |
|---|---|---|
| `Marshaler` | `Marshal(map[int32]*procutil.StatsWithPerm) ([]byte, error)`, `ContentType() string` | Serializes a PID-keyed stats map |
| `Unmarshaler` | `Unmarshal([]byte) (*model.ProcStatsWithPermByPID, error)` | Deserializes bytes back into a PID-keyed stats map |

Both interfaces are implemented by `protoSerializer` and `jsonSerializer`.

### Key types

#### Content type constants

| Constant | Value | Defined in |
|---|---|---|
| `ContentTypeProtobuf` | `"application/protobuf"` | `protobuf.go` |
| `ContentTypeJSON` | `"application/json"` | `json.go` |

### Key functions

#### Serializer selection

**`GetMarshaler(accept string) Marshaler`** — returns `protoSerializer` if the `Accept` header contains `"application/protobuf"`, otherwise `jsonSerializer`.

**`GetUnmarshaler(ctype string) Unmarshaler`** — same logic against the `Content-Type` header.

#### Protobuf serializer (`protobuf.go`)

Uses `github.com/gogo/protobuf/proto`. Fields mapped from `procutil.StatsWithPerm` to `model.ProcStatsWithPerm`:
- `OpenFdCount`, `IOStat.ReadCount`, `IOStat.WriteCount`, `IOStat.ReadBytes`, `IOStat.WriteBytes`

The serializer uses a **typed object pool** (`statPool`, backed by `ddsync.NewDefaultTypedPool`) to reuse `model.ProcStatsWithPerm` allocations; `returnToPool` is called after marshaling to return pooled objects.

#### JSON serializer (`json.go`)

Uses `github.com/gogo/protobuf/jsonpb` with `EmitDefaults: true` so zero-valued fields are included. Same field mapping and pool usage as the protobuf path.

### Configuration and build flags

No special build tags. The format is negotiated at runtime via HTTP `Accept`/`Content-Type` headers. Protobuf is the production default; JSON is a fallback for tests.

#### Request sub-package (`encoding/request`)

Mirrors the top-level package but operates on `pbgo.ProcessStatRequest` (the request type process-agent sends to system-probe):

- `Marshaler` / `Unmarshaler` interfaces with the same negotiation helpers (`GetMarshaler`, `GetUnmarshaler`)
- Uses `google.golang.org/protobuf/encoding/protojson` (not gogo/protobuf) for JSON

## Usage

**system-probe side** (`cmd/system-probe/modules/process.go`):

```go
marshaler := encoding.GetMarshaler(contentType)  // contentType from Accept header
buf, err  := marshaler.Marshal(statsMap)
w.Header().Set("Content-Type", marshaler.ContentType())
w.Write(buf)
```

**process-agent side** (`pkg/process/net/common.go` and tests):

```go
unmarshaler := encoding.GetUnmarshaler(resp.Header.Get("Content-Type"))
stats, err  := unmarshaler.Unmarshal(body)
```

The default path in production uses Protobuf because the process-agent sets `Accept: application/protobuf`. JSON is used in tests and as a fallback when the header is absent or unrecognized.
