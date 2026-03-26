# pkg/logs/internal

## Purpose

`pkg/logs/internal` contains the log-pipeline building blocks that transform raw byte streams from tailers (file, socket, Docker, Kubernetes, etc.) into structured `message.Message` values ready for the sender. It is an internal package — callers outside `pkg/logs` and `comp/logs` must not import it directly.

The pipeline order is:

```
Tailer bytes
  → Framer       (break stream into frames by newline / Docker header / encoding)
  → LineParser   (handle partial lines, apply a source-specific Parser)
  → Preprocessor (JSON aggregation → tokenization → labeling → multiline aggregation → sampling)
  → outputChan   (message.Message)
```

Each stage is wired together in `decoder/decoder.go`.

---

## Sub-packages

### `decoder/`

Owns the top-level pipeline actor and all its stages.

| Symbol | Description |
|---|---|
| `Decoder` interface | `Start()`, `Stop()`, `InputChan()`, `OutputChan()`, `GetLineCount()`, `GetDetectedPattern()`. The actor's public contract — tailers write raw `*message.Message` to `InputChan`; processed messages arrive on `OutputChan`. |
| `InitializeDecoder(source, parser, tailerInfo)` | Creates a `Decoder` with UTF-8 newline framing. The standard entry point for file and socket tailers. |
| `NewDecoderWithFraming(source, parser, framing, multiLinePattern, tailerInfo)` | Creates a `Decoder` with an explicit `framer.Framing`. Selects the line handler based on source config (user regex, auto multiline, detection-only, or pass-through). |
| `NewNoopDecoder()` | A pass-through decoder used in tests or when no processing is needed. |
| `LineHandler` interface | Internal `process(*message.Message)`, `flushChan()`, `flush()`. Implemented by `SingleLineHandler`, `MultiLineHandler`, `LegacyAutoMultilineHandler`, and `preprocessorLineHandler`. |
| `LineParser` interface | Wraps a `parsers.Parser`, handles partial-line assembly for sources like Kubernetes. Two implementations: `SingleLineParser` and `MultiLineParser`. |
| `DetectedPattern` | Thread-safe container for the regex detected by auto-multiline, surfaced back to the tailer for reuse after file rotation. |

#### `decoder/preprocessor/`

Implements the modern multiline detection and aggregation pipeline.

| Symbol | Description |
|---|---|
| `Preprocessor` | Orchestrates five stages in order: JSON aggregation → tokenization → labeling → aggregation → sampling. |
| `Tokenizer` | Converts raw log bytes into a sequence of `Token` values representing structural features (timestamps, log levels, JSON braces, etc.) used by the labeler. |
| `Labeler` / `NoopLabeler` | Assigns a `Label` (e.g. `StartGroup`, `NoGroup`) to a log line based on token features. |
| `Aggregator` interface | Four implementations: `PassThroughAggregator` (no combining), `RegexAggregator` (user-configured regex), `CombiningAggregator` (auto multiline with combining), `DetectingAggregator` (tagging without combining). |
| `JSONAggregator` / `NoopJSONAggregator` | Reassembles multi-chunk JSON log entries before the rest of the pipeline. Enabled via `logs_config.auto_multi_line.enable_json_aggregation`. |
| `Sampler` / `NoopSampler` | Optional per-source sampling stage. |

---

### `framer/`

Breaks a continuous byte stream into discrete frames before passing them to the line parser.

| Symbol | Description |
|---|---|
| `Framer` | Stateful struct. `Process(*message.Message)` feeds new bytes; calls `outputFn` for each complete frame. Maintains an internal buffer to handle partial frames across calls. Enforces `contentLenLimit`; over-length content is truncated and flagged. |
| `NewFramer(outputFn, framing, contentLenLimit)` | Constructor. Selects the `FrameMatcher` based on `framing`. |
| `Framing` (int enum) | `UTF8Newline`, `UTF16BENewline`, `UTF16LENewline`, `SHIFTJISNewline`, `NoFraming`, `DockerStream`. |
| `FrameMatcher` interface | `FindFrame(buf []byte, seen int) (content []byte, rawDataLen int, isTruncated bool)`. Implemented per encoding. |

---

### `parsers/`

Transforms a raw frame into a structured `message.Message` with timestamp, severity, and content.

| Symbol | Description |
|---|---|
| `Parser` interface | `Parse(*message.Message) (*message.Message, error)` + `SupportsPartialLine() bool`. |

Available parser implementations:

| Package | Parser | Input format |
|---|---|---|
| `parsers/kubernetes` | `kubernetes.New()` | Kubernetes CRI log format: `<RFC3339Nano> <stream> <P\|F> <content>`. Sets `IsPartial` from the `P`/`F` flag. |
| `parsers/dockerstream` | `dockerstream.New(containerID)` | Docker multiplexed log stream: 8-byte header + timestamp + content. Strips partial-chunk headers for messages >16 KB. |
| `parsers/dockerfile` | `dockerfile.New()` | Docker JSON file log driver format. |
| `parsers/encodedtext` | `encodedtext.New(enc)` | Newline-delimited text in UTF-16-BE, UTF-16-LE, or Shift-JIS; re-encodes to UTF-8. |
| `parsers/integrations` | `integrations.New()` | Datadog integration check log format (severity prefix). |
| `parsers/noop` | `noop.New()` | Pass-through; returns the message unchanged. |

---

### `tag/`

Provides tag lists attached to log messages.

| Symbol | Description |
|---|---|
| `Provider` interface | `GetTags() []string`. |
| `NewProvider(entityID, tagAdder)` | Returns a `provider` that queries the Tagger for the entity's high-cardinality tags, with a one-time tagger warm-up sleep on first call. |
| `NewLocalProvider(tags []string)` | Returns a `localProvider` for statically configured tags. Supports an optional `expected_tags` window (controlled by `logs_config.expected_tags_duration`) that includes host-level system tags until the deadline passes. |

---

### `util/`

Miscellaneous helpers.

| Sub-package / file | Purpose |
|---|---|
| `util/adlistener/` | `ADListener` — a thin `autodiscovery/scheduler.Scheduler` adapter that proxies `Schedule`/`Unschedule` calls to caller-supplied functions. Used by `schedulers/ad`. |
| `util/containersorpods/` | Determines at runtime whether the agent is running alongside containers or pods, driving decisions about log collection strategies. Build-tagged implementations for Docker and Kubernetes. |
| `util/opener/` | Platform-specific file-open helpers (`open_linux.go`, `open_other.go`). |
| `util/service_name.go` | `ServiceNameFromTags(ctrName, entityID, taggerFunc)` — extracts the `service:` standard tag from the Tagger for a container. |
| `util/moving_sum.go` | `MovingSum` — a time-windowed counter used by the legacy auto-multiline handler to track match rates. |

---

## Configuration knobs (relevant `datadog.yaml` keys)

| Key | Effect |
|---|---|
| `logs_config.auto_multi_line_detection_tagging` | Enable detection-only multiline mode (tags group starts, does not combine). |
| `logs_config.auto_multi_line.enable_json_aggregation` | Combine split JSON objects into a single log message. |
| `logs_config.auto_multi_line.tokenizer_max_input_bytes` | Maximum bytes fed to the tokenizer per line. |
| `logs_config.expected_tags_duration` | How long to include host-level expected tags on logs after agent start. |
| `logs_config.add_logsource_tag` | Add a `logsource:stdout` / `logsource:stderr` tag from Docker/Kubernetes parsers. |

## Usage

### Creating a decoder in a file tailer

```go
// Standard UTF-8 newline-framed decoder (file, socket tailers)
dec := decoder.InitializeDecoder(source, parsers.Noop, tailerInfo)
dec.Start()

// Write raw bytes into the decoder
dec.InputChan() <- message.NewInput(rawBytes)

// Read decoded messages from the decoder
for msg := range dec.OutputChan() {
    msg.Origin = origin
    pipelineChan <- msg
}

dec.Stop()
```

### Creating a decoder with explicit framing (container tailers)

```go
// Docker multiplexed stream (8-byte header per frame)
dec := decoder.NewDecoderWithFraming(
    source,
    parsers.DockerStream("containerID"),
    framer.DockerStream,
    nil,   // no user-defined multiline pattern
    tailerInfo,
)
```

### Choosing a parser

| Source type | Parser |
|---|---|
| Plain file / socket | `parsers/noop.New()` |
| Kubernetes CRI log file | `parsers/kubernetes.New()` |
| Docker socket stream | `parsers/dockerstream.New(containerID)` |
| Docker JSON log file | `parsers/dockerfile.New()` |
| UTF-16 / Shift-JIS text file | `parsers/encodedtext.New(enc)` |
| Integration check log | `parsers/integrations.New()` |

### Adding a new framing strategy

1. Add a `Framing` constant in `framer/framing.go`.
2. Implement `FrameMatcher` and register it in `framer.NewFramer`'s switch.
3. Pass the new constant to `decoder.NewDecoderWithFraming`.

### Adding a new parser

1. Implement `parsers.Parser` (two methods: `Parse(*message.Message)` and `SupportsPartialLine() bool`).
2. Create the parser in the launcher that handles the new source type and pass it to `decoder.InitializeDecoder` or `decoder.NewDecoderWithFraming`.

## Related documentation

| Document | Relationship |
|---|---|
| [logs.md](logs.md) | Top-level overview; `internal/decoder` is listed under the "How to log" half of the pipeline between tailers and the message channel |
| [tailers.md](tailers.md) | File and socket tailers call `decoder.InitializeDecoder` / `decoder.NewDecoderWithFraming` to create a `Decoder`; the decoder's `InputChan` receives raw `*message.Message` bytes and its `OutputChan` produces decoded messages that the tailer forwards to the pipeline |
| [message.md](message.md) | `*message.Message` is both the input type of the decoder (raw, `StateUnstructured`) and its output type (decoded, with `Origin` and `Status` populated); `TruncatedFlag`, `EscapedLineFeed`, and `AggregatedJSONTag` are sentinel values produced by the decoder stages |
| [launchers.md](launchers.md) | Launchers determine which parser to pass to `InitializeDecoder` based on the source type (e.g. `parsers/kubernetes` for CRI logs, `parsers/dockerstream` for Docker socket streams, `parsers/noop` for plain file tailing) |
| [sources.md](sources.md) | `sources.LogSource.Config` carries the multiline pattern and auto-multiline settings that `NewDecoderWithFraming` reads to select the `LineHandler` implementation; `ReplaceableSource` lets a tailer swap its `LogSource` without recreating the decoder |
