# anomaly_detection_investigator

A small CLI tool that watches for anomaly events emitted by the agent Q-branch observer and optionally triggers Bits AI investigations on them.

## How it works

The tool polls the Datadog Events API for events tagged `source:agent-q-branch-observer`, batches them, and either displays them or triggers a Bits AI investigation.

**Two modes:**

- **Live mode** (default): polls on a configurable interval, accumulates events, and flushes the batch when idle (no new events in a poll) or when the max duration is exceeded.
- **One-shot mode** (`--from`/`--to`): fetches events over a fixed historical time range, displays them, and exits.

## Prerequisites

```bash
export DD_API_KEY=<your-api-key>
export DD_APP_KEY=<your-app-key>
```

## Usage

```bash
go run ./cmd/anomaly_detection_investigator [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-interval` | `300` | Seconds between API polls (also the idle window to trigger a flush) |
| `-max-duration` | `840` | Max seconds to accumulate before a forced flush |
| `-run_bits` | `false` | Trigger a Bits AI investigation when a batch is ready (default: display only) |
| `-from` | — | RFC3339 start time for one-shot mode (e.g. `2026-03-13T10:00:00Z`) |
| `-to` | — | RFC3339 end time for one-shot mode (requires `-from`) |

### Examples

Watch live, display batches only (dry run):
```bash
go run ./cmd/anomaly_detection_investigator
```

Watch live and trigger Bits AI on each batch:
```bash
go run ./cmd/anomaly_detection_investigator -run_bits
```

Fetch a historical window and trigger a Bits AI investigation:
```bash
go run ./cmd/anomaly_detection_investigator \
  -from 2026-03-13T10:00:00Z \
  -to   2026-03-13T11:00:00Z \
  -run_bits
```

## Output

When a batch is flushed the tool prints the event messages and, if `-run_bits` is set, posts a `general_investigation` to the Bits AI API and prints the resulting investigation URL:

```
Investigation URL: https://dddev.datadoghq.com/bits-ai/investigations/<id>?section=conclusion&v=trace
```

Without `-run_bits` all investigation payloads are printed as a dry run instead of sent.
