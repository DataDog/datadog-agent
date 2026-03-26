# pkg/logs/diagnostic

## Purpose

`pkg/logs/diagnostic` provides a runtime diagnostic tap for the log pipeline. When enabled, every log message processed by the pipeline is intercepted, optionally filtered, and streamed to a consumer (typically the CLI via `agent stream-logs`). This lets operators inspect the exact messages flowing through the agent in real time without changing any configuration or restarting the agent.

The package is intentionally lightweight: it inserts no overhead into the hot path when diagnostics are disabled.

## Key elements

### Interfaces

| Name | Description |
|------|-------------|
| `MessageReceiver` | Single-method interface (`HandleMessage(*message.Message, []byte, string)`) implemented by any type that wants to receive pipeline messages. The log `Processor` accepts a `MessageReceiver` and calls it for each processed message. |
| `Formatter` | Converts a `message.Message` plus the already-rendered (redacted) bytes and an event-type string into a human-readable string for display. |

### Types

| Name | File | Description |
|------|------|-------------|
| `BufferedMessageReceiver` | `message_receiver.go` | Concrete, thread-safe implementation of `MessageReceiver`. Internally uses a buffered channel (`logs_config.message_channel_size` deep) and an enable/disable flag protected by a `sync.RWMutex`. Inactive by default; must be explicitly enabled with `SetEnabled(true)`. |
| `NoopMessageReceiver` | `noop_message_receiver.go` | Drop-everything implementation used in tests or when diagnostics are not needed. |
| `Filters` | `message_receiver.go` | Struct with four optional string fields (`Name`, `Type`, `Source`, `Service`) used to narrow which messages are forwarded to the consumer. An empty filter passes all messages. |
| `logFormatter` (unexported) | `format.go` | Default `Formatter` that emits a one-line string: `Integration Name | Type | Status | Timestamp | Hostname | Service | Source | Tags | Message`. |

### Key functions and methods

| Signature | Description |
|-----------|-------------|
| `NewBufferedMessageReceiver(f Formatter, hostname hostnameinterface.Component) *BufferedMessageReceiver` | Constructor. Pass `nil` for `f` to use the default `logFormatter`. |
| `(*BufferedMessageReceiver).SetEnabled(bool) bool` | Toggles collection on/off. Returns `true` if the state actually changed. Disabling clears buffered messages. |
| `(*BufferedMessageReceiver).HandleMessage(m, rendered, eventType)` | Called by the pipeline processor for each message. No-ops immediately if not enabled. |
| `(*BufferedMessageReceiver).Filter(filters *Filters, done <-chan struct{}) <-chan string` | Launches a goroutine that reads from the internal channel, applies `filters`, formats matching messages, and writes them to the returned string channel. Close `done` to stop the goroutine. |
| `(*BufferedMessageReceiver).Start()` / `Stop()` | Re-creates / closes the internal channel; call these in step with the pipeline lifecycle. |

## Usage

### How the pipeline wires it in

`pkg/logs/processor.Processor` accepts a `MessageReceiver` at construction time. After processing each message (redacting, tagging, encoding), it calls `receiver.HandleMessage(msg, renderedBytes, eventType)`. This happens for every message regardless of whether the receiver is enabled; the enable check is inside `HandleMessage` and is a cheap atomic read.

```
logs pipeline
  └── Processor
        └── receiver.HandleMessage(msg, rendered, eventType)
              └── BufferedMessageReceiver.inputChan <- messagePair{...}  (if enabled)
```

### CLI integration (`agent stream-logs`)

`cmd/agent/subcommands/streamlogs` exposes the `agent stream-logs` command. It sends an HTTP request to the running agent's IPC endpoint, which in turn calls `SetEnabled(true)`, then opens an SSE-style stream backed by `Filter(filters, done)`. When the command exits (or `--duration` elapses), it closes the `done` channel and calls `SetEnabled(false)`.

Filter flags map directly to `Filters` fields:

| Flag | Filters field |
|------|---------------|
| `--name` | `Name` (log source name) |
| `--type` | `Type` (source config type, e.g. `file`, `docker`) |
| `--source` | `Source` (log source tag) |
| `--service` | `Service` (service tag) |

### Providing a custom formatter

Pass a type that implements `Formatter` to `NewBufferedMessageReceiver`. The interface has one method:

```go
Format(m *message.Message, eventType string, rendered []byte) string
```

`rendered` is the already-scrubbed/redacted content byte slice. Prefer it over `m.Content` when displaying message text.
