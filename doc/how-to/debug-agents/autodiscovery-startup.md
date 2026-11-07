# Autodiscovery startup traces

Set `DD_FX_TRACING_ENABLED=true` on an Agent with Fx instrumentation enabled to
send opt-in startup traces to its local trace-agent. No additional tracing or
configuration-file contents are collected when Fx tracing is disabled.

Search APM for:

```text
service:dd-agent-fx-init operation_name:autodiscovery.startup resource_name:autodiscovery
```

Autodiscovery has a separate startup root because its background preparation can
continue after Fx Started. The existing `operation_name:startup` root still ends
at Fx Started. The diagnostic root begins when preparation is requested and ends
after `LoadAndRun` joins preparation and performs the providers' initial loads.
Its `fx.trace_id` attribute links it to the Fx trace, `process_name` identifies
the Agent process, and `agent.version` records the Agent build. Children inherit
these attributes so individual phases can be grouped by process/build. Shutdown
before successful startup finishes the diagnostic trace with an error after
joining outstanding preparation.

Children use `operation_name:autodiscovery.phase` and stable resource names:

| Resource | Work measured |
| --- | --- |
| `preparation` | Default provider/listener configuration, running in the preparation goroutine. |
| `config.initialize` | Config reader initialization, including lock acquisition and its first scan. |
| `config.scan` | Directory enumeration and concurrent config-file processing. |
| `providers.discover` | Provider/listener discovery from configuration and environment. |
| `provider.init` | One provider factory invocation; `provider` identifies it. |
| `listeners.initialize` | Initial listener setup, including lock acquisition. |
| `listener.init` | One initial listener factory and successful `Listen` call; `listener` identifies it. Later retries are excluded. |
| `preparation.wait` | Time the `LoadAndRun` caller waits for preparation; cancellation marks the span as an error. |
| `providers.load` | Initial provider startup after preparation. |
| `provider.load` | One provider's initial collection or streaming initialization; `provider` identifies it. |

`config.scan` carries numeric attributes:

- `config.files`: file-read attempts, including invalid or unreadable configs.
- `config.bytes`: bytes read.
- `config.loaded`, `config.errors`: returned configuration/error counts.
- `config.worker_limit`: configured worker limit, clamped to at least one. A
  directory with fewer entries uses fewer workers.
- `config.enumerate_time_ns`, `config.read_time_ns`, `config.parse_time_ns`,
  `config.format_time_ns`, `config.hash_time_ns`: accumulated worker timings in
  nanoseconds. Format time covers serialization of the full config format and
  scrubbing, and hash time covers hashing that format. Normalization/marshaling
  of individual check instances remains part of the overall scan duration.

These worker totals overlap when several workers run concurrently; they are not
CPU-time measurements and must not be added to reconstruct wall latency. Use the
span duration for elapsed scan time. `config.with_formats` indicates whether the
scan builds scrubbed formats. The normal initial scan skips this work; a later
metadata request or cache refresh may still build formats outside this trace.

The trace records phases and aggregate counters, not per-file spans, filenames,
or configuration values. It is sent asynchronously on completion using the
existing Fx tracing transport. A process that exits before completion/delivery
can be absent, and APM retention can sample the trace. This is diagnostic timing,
not a complete fleet startup KPI or an assertion of first-payload delivery.
