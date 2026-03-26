> **TL;DR:** `pkg/logs/message` defines the core data types (`Message`, `Payload`, `Origin`) that a log line carries through every stage of the pipeline, from raw tailer bytes through decoding, processing, encoding, and final delivery.

# pkg/logs/message

## Purpose

`pkg/logs/message` defines the core data structures that a log line carries through the entire
logs pipeline — from the moment a tailer reads raw bytes off a file or socket, through decoding,
processing, encoding, and final delivery to the Datadog intake.

Every component in `pkg/logs` (decoder, processor, sender, destination) operates on
`*message.Message` or `*message.Payload`. Nothing outside the package needs to know how a
message is transported; the types here provide a stable contract between all pipeline stages.

## Key elements

### Key types

#### `Message`

The central type: a log line together with all of its metadata.

```go
type Message struct {
    MessageContent   // content + state machine
    MessageMetadata  // hostname, origin, status, timestamps, tags, …
}
```

`Message` is always allocated on the heap (`*Message`) and flows through Go channels.

#### `MessageContent` and the state machine

`MessageContent` tracks how the content bytes have been transformed. The state field drives
`GetContent` and `SetContent` so callers do not need to know which representation is active.

```go
type MessageContentState uint32

const (
    StateUnstructured MessageContentState = iota  // raw bytes from a tailer (file, socket, …)
    StateStructured                               // structured object from journald / Windows Event Log
    StateRendered                                 // content rendered to bytes, ready to encode
    StateEncoded                                  // content encoded (JSON/proto/raw), ready to send
)
```

State transitions:

```
StateUnstructured ──┐
                    ├──► (Processor.Render) ──► StateRendered ──► (Encoder.Encode) ──► StateEncoded
StateStructured  ───┘
```

Key methods:

```go
func (m *MessageContent) GetContent() []byte       // returns the "message body" regardless of state
func (m *MessageContent) SetContent([]byte)        // updates the body for the current state
func (m *MessageContent) SetRendered([]byte)       // transitions to StateRendered
func (m *MessageContent) SetEncoded([]byte)        // transitions to StateEncoded
func (m *Message) Render() ([]byte, error)         // renders structured → bytes; no-op for others
```

**Important:** always use `GetContent`/`SetContent` rather than accessing `content` directly.
For `StateStructured`, `GetContent` returns only the user-visible message field (e.g. from a
journald entry), not the full structured object.

### Key interfaces

#### `StructuredContent` interface

Implemented by tailers that emit structured log objects (journald, Windows Event Log).

```go
type StructuredContent interface {
    Render() ([]byte, error)  // serializes the full object (e.g. JSON)
    GetContent() []byte       // returns only the human-readable message portion
    SetContent([]byte)        // updates the message portion in-place
}
```

`BasicStructuredContent` is the default implementation, backed by `map[string]interface{}` with
a `"message"` key.

#### `MessageMetadata`

All non-content metadata attached to a message.

```go
type MessageMetadata struct {
    Hostname           string
    Origin             *Origin          // source, service, tags, log source reference
    Status             string           // severity ("info", "warn", "error", …)
    IngestionTimestamp int64            // Unix nanoseconds, set at decode time
    RawDataLen         int              // original byte length before any trimming
    ProcessingTags     []string         // tags added by the processor (redaction rules, MRF, …)
    ParsingExtra                        // partial/truncated/multiline flags, docker timestamp offset
    ServerlessExtra                     // optional timestamp override for serverless
}
```

Convenience methods:

```go
func (m *MessageMetadata) GetStatus() string          // returns StatusInfo if Status is empty
func (m *MessageMetadata) Tags() []string             // all tags from origin + processing
func (m *MessageMetadata) TagsToString() string       // comma-separated tag string
func (m *MessageMetadata) GetLatency() int64          // nanoseconds since ingestion
func (m *MessageMetadata) RecordProcessingRule(ruleType, ruleName string)
```

#### `Origin`

Links a message to its `sources.LogSource` and carries per-message overrides for service,
source, and tags.

```go
type Origin struct {
    LogSource   *sources.LogSource
    Identifier  string        // container/pod identifier
    Offset      string        // tailer read offset (for the auditor)
    FilePath    string        // concrete file path (fingerprinting)
    Fingerprint *types.Fingerprint
}

func NewOrigin(source *sources.LogSource) *Origin
func (o *Origin) Source() string          // Config.Source wins over per-message override
func (o *Origin) Service() string         // Config.Service wins over per-message override
func (o *Origin) SetSource(string)
func (o *Origin) SetService(string)
func (o *Origin) SetTags([]string)
func (o *Origin) Tags(processingTags []string) []string
func (o *Origin) TagsPayload(processingTags []string) []byte   // syslog-style tag header bytes
```

### Configuration and build flags

#### Status constants and syslog severity

```go
const (
    StatusEmergency = "emergency"
    StatusAlert     = "alert"
    StatusCritical  = "critical"
    StatusError     = "error"
    StatusWarning   = "warn"
    StatusNotice    = "notice"
    StatusInfo      = "info"
    StatusDebug     = "debug"
)

func StatusToSeverity(status string) []byte   // returns syslog severity bytes (e.g. "<46>")
```

### Key functions

#### `Payload`

A batch of encoded messages ready to be sent to the intake (output of `sender.Strategy`).

```go
type Payload struct {
    MessageMetas  []*MessageMetadata  // one entry per original message
    Encoded       []byte              // possibly compressed bytes to send
    Encoding      string              // HTTP Content-Encoding header; empty for TCP
    UnencodedSize int
}

func NewPayload(messageMetas []*MessageMetadata, encoded []byte, encoding string, unencodedSize int) *Payload
func (m *Payload) Count() int64    // number of messages in the payload
func (m *Payload) Size() int64     // sum of RawDataLen across all messages
func (m *Payload) IsMRF() bool     // true if all messages should go to MRF destinations
```

#### Constructor functions

| Function | Use case |
|---|---|
| `NewMessage(content, origin, status, ts)` | General purpose unstructured message |
| `NewMessageWithSource(content, status, source, ts)` | Shorthand; wraps `NewOrigin(source)` |
| `NewMessageWithSourceWithParsingExtra(...)` | Same but also sets `IsTruncated` |
| `NewMessageWithParsingExtra(...)` | Unstructured with full `ParsingExtra` control |
| `NewStructuredMessage(content, origin, status, ts)` | Structured message (journald, WinEvent) |
| `NewStructuredMessageWithParsingExtra(...)` | Structured with `IsTruncated` |

#### Sentinel values and tag helpers

```go
var TruncatedFlag = []byte("...TRUNCATED...")  // prepended/appended to truncated lines
var EscapedLineFeed = []byte(`\n`)             // newline escape for multiline transport

const AggregatedJSONTag = "aggregated_json:true"  // added to recombined JSON messages

func TruncatedReasonTag(reason string) string   // "truncated:<reason>"
func MultiLineSourceTag(source string) string   // "multiline:<source>"
func LogSourceTag(stream string) string         // "logsource:stdout" / "logsource:stderr"
```

#### `ParsingExtra`

Extra fields populated by parsers and consumed by downstream stages.

```go
type ParsingExtra struct {
    Timestamp   string   // docker log timestamp; used as tailer offset
    IsPartial   bool     // docker partial log flag
    IsTruncated bool     // line was truncated at the decoder level
    IsMultiLine bool     // message was assembled from multiple lines
    IsMRFAllow  bool     // set by the processor for MRF routing
    Tags        []string // per-message tags from the parser (e.g. container stream)
}
```

## Related documentation

| Document | Relationship |
|---|---|
| [logs.md](logs.md) | Top-level overview of the logs pipeline; shows where `*message.Message` fits in the full data flow |
| [sources.md](sources.md) | `LogSource` is embedded in `Origin`; every `Message` links back to the source that produced it |
| [pipeline.md](pipeline.md) | The `Pipeline` is the consumer of `*message.Message` channels and the producer of `*message.Payload` |
| [processor.md](processor.md) | The `Processor` drives the state machine: calls `Render()`, `SetRendered()`, `Encode()`, `SetEncoded()` |

## Usage

### Creating a message in a tailer

```go
// Unstructured (file, socket)
msg := message.NewMessageWithSource(
    lineBytes,
    message.StatusInfo,
    logSource,
    time.Now().UnixNano(),
)
pipelineChan <- msg

// Structured (journald entry)
content := &message.BasicStructuredContent{
    Data: map[string]interface{}{
        "message":   entry.Fields["MESSAGE"],
        "SYSLOG_IDENTIFIER": entry.Fields["SYSLOG_IDENTIFIER"],
    },
}
msg := message.NewStructuredMessage(content, origin, message.StatusInfo, time.Now().UnixNano())
pipelineChan <- msg
```

### Reading a message in the processor

```go
body := msg.GetContent()              // safe regardless of state
msg.SetContent(redacted)              // write back after scrubbing

rendered, _ := msg.Render()           // collapses structured → bytes
msg.SetRendered(rendered)             // advances state to StateRendered

encoder.Encode(msg, hostname)         // expects StateRendered; advances to StateEncoded
```

### Accessing origin metadata

```go
service := msg.Origin.Service()       // respects Config.Service override
source  := msg.Origin.Source()        // respects Config.Source override
tags    := msg.Tags()                 // merges origin tags + processing tags
```

### Status and severity

```go
status := msg.GetStatus()             // defaults to StatusInfo if empty
sev    := message.StatusToSeverity(status)   // []byte for TCP syslog framing
```

### In the JSON encoder

The processor's `jsonEncoder` builds the final JSON payload from the rendered message:

```go
json.Marshal(jsonPayload{
    Message:   ValidUtf8Bytes(msg.GetContent()),
    Status:    msg.GetStatus(),
    Timestamp: ts.UnixNano() / nanoToMillis,
    Hostname:  hostname,
    Service:   msg.Origin.Service(),
    Source:    msg.Origin.Source(),
    Tags:      msg.TagsToString(),
})
msg.SetEncoded(encoded)
```

### In the decoder

The decoder constructs bare input messages (no origin or status) and fills them in as framing
and parsing complete:

```go
func NewInput(content []byte) *message.Message {
    return message.NewMessage(content, nil, "", time.Now().UnixNano())
}
```

Origin and status are attached later by the tailer before the message enters the pipeline.

### Lifecycle across pipeline stages

| Stage | State before | Action | State after |
|-------|-------------|--------|-------------|
| Tailer (file/socket) | — | `NewMessageWithSource` | `StateUnstructured` |
| Tailer (journald/WinEvent) | — | `NewStructuredMessage` | `StateStructured` |
| Processor — render | `StateUnstructured` / `StateStructured` | `msg.Render()` + `msg.SetRendered(rendered)` | `StateRendered` |
| Processor — encode | `StateRendered` | `encoder.Encode(msg, hostname)` | `StateEncoded` |
| Strategy / Sender | `StateEncoded` | `msg.GetContent()` to build `Payload.Encoded` | (consumed) |

The `Payload` type groups the `MessageMetadata` slice (one per original message) with the compressed wire bytes. Both `Payload.Count()` and `Payload.Size()` are derived from the metadata slice and are used by the sender for telemetry and auditing.
