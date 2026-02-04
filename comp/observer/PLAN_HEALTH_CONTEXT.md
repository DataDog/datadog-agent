# Plan: Health Score + Context Packets

## Goal
When anomaly detected → capture context packet (metrics + logs + events) for flare/debugging.

---

## Current State

```
Metrics → Parquet storage → Replay ✅
Logs → Converted to metrics only, raw logs discarded
Events → Loaded from JSON in testbench only
```

**Gap**: Raw logs not buffered in agent, can't include in context packet.

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Observer                           │
│                                                         │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐            │
│  │ Metrics  │   │   Logs   │   │  Events  │            │
│  │ Storage  │   │  Buffer  │   │  Buffer  │            │
│  │(parquet) │   │ (ring)   │   │ (ring)   │            │
│  └────┬─────┘   └────┬─────┘   └────┬─────┘            │
│       │              │              │                   │
│       └──────────────┼──────────────┘                   │
│                      ▼                                  │
│              ┌───────────────┐                          │
│              │ Health Score  │ ◄─── anomaly count,      │
│              │   Calculator  │      severity, recency   │
│              └───────┬───────┘                          │
│                      │                                  │
│                      ▼ (on anomaly or threshold)        │
│              ┌───────────────┐                          │
│              │Context Packet │                          │
│              │   Generator   │                          │
│              └───────┬───────┘                          │
│                      │                                  │
│                      ▼                                  │
│              ┌───────────────┐                          │
│              │ context_packets/                         │
│              │   <timestamp>.json                       │
│              └───────────────┘                          │
└─────────────────────────────────────────────────────────┘
```

---

## Components to Build

### 1. Smart Log Buffer (Minimal Memory)
Only keep "high value" logs, dedup by pattern.

```go
type LogBuffer struct {
    // Deduped by signature - one example per pattern
    patternLogs map[string]*PatternBucket

    // Always keep error/warn regardless of dedup
    errorLogs   []BufferedLog

    maxPatterns int           // e.g., 500 unique patterns
    maxErrors   int           // e.g., 200 error logs
    windowSec   int           // e.g., 60s, evict older
}

type PatternBucket struct {
    Signature   string
    Example     BufferedLog   // one representative log
    Count       int           // how many times seen
    FirstSeen   time.Time
    LastSeen    time.Time
}

type BufferedLog struct {
    Timestamp int64
    Content   []byte
    Tags      []string
    Source    string
    Level     string        // error, warn, info, debug
}
```

**High-value log criteria:**
- Error/warn level → always keep (up to maxErrors)
- New pattern → keep as example
- Existing pattern → just increment count, don't store

**Why this works:**
- Already have `logSignature()` function for pattern extraction
- Instead of 10k logs, keep ~500 patterns + 200 errors = much smaller
- Context packet shows: "saw this pattern 847 times" + one example

**Integration**: Hook into existing `ObserveLog()` path.

### 2. Health Score Calculator
Single 0-100 number based on recent anomalies.

```go
type HealthScore struct {
    Score       int       // 0-100, 100 = healthy
    LastUpdated time.Time
    Factors     []HealthFactor
}

type HealthFactor struct {
    Name   string  // "anomaly_count", "severity", "correlation_size"
    Value  float64
    Weight float64
}
```

**Formula (strawman)**:
```
score = 100 - (anomaly_count * 5) - (max_severity * 10) - (cluster_size * 2)
clamped to [0, 100]
```

### 3. Context Packet
Snapshot captured when health drops below threshold.

```go
type ContextPacket struct {
    ID              string
    Timestamp       time.Time

    // Health
    HealthBefore    int
    HealthAfter     int
    HealthDrop      int               // how much it dropped

    // Trigger
    TriggerAnomaly  AnomalyOutput

    // Correlation
    BlastRadius     []AnomalyOutput   // TimeCluster
    CausalChain     []LeadLagEdge     // LeadLag

    // Logs (smart buffer output)
    LogPatterns     []LogPatternSummary  // deduped patterns with counts
    ErrorLogs       []BufferedLog        // actual error/warn logs

    // Other context
    Metrics         []MetricSnapshot  // related series
    Events          []EventSignal     // deploys, restarts

    // Meta
    PatternType     string            // "memory_leak", "crash_loop", etc.
    SuggestedAction string
}

type LogPatternSummary struct {
    Signature  string
    Example    string       // one representative log line
    Count      int          // times seen in window
    FirstSeen  time.Time
    LastSeen   time.Time
    Sources    []string     // which services/sources
}
```

### 4. Flare Integration
Include recent context packets in `agent flare`.

```
flare/
  observer/
    health_score.json        # current health + history
    context_packets/
      2024-01-15T10-32-00.json
      2024-01-15T10-45-12.json
    metrics_summary.json     # recent anomaly stats
```

---

## Implementation Phases (TDD Order)

### Phase 0: Log Recording + Replay (FIRST)
Build testability before features.

**Current state:**
- Metrics recording: ✅ `observer.capture_metrics.enabled` → parquet
- Metrics replay: ✅ `ReadAllMetrics()` in testbench
- Log recording: ❌ TODO in `recorder.go:173-176`
- Log replay: ✅ testbench loads from `logs/` dir (if data existed)
- Existing scenarios: ❌ No logs (only parquet metrics)

**0a. Add Log Recording to Recorder** (the missing piece)
File: `comp/anomalydetection/recorder/impl/recorder.go`

```go
// Currently:
func (h *recordingHandle) ObserveLog(msg observer.LogView) {
    h.inner.ObserveLog(msg)
    // TODO: Optionally record logs to parquet (future enhancement)
}

// Change to:
func (h *recordingHandle) ObserveLog(msg observer.LogView) {
    h.inner.ObserveLog(msg)
    if h.recorder.logWriter != nil {
        h.recorder.logWriter.WriteLog(msg)
    }
}
```

- [ ] Add `LogWriter` - writes logs to JSON lines file
- [ ] Config: `observer.capture_logs.enabled`, `observer.logs_output_file`
- [ ] Format: `{"timestamp":X,"content":"...","tags":[...],"source":"...","level":"..."}`
- [ ] Hook into existing `ObserveLog()` in recorder

**0b. Store Raw Logs in Testbench** (keep after load for API)
File: `comp/observer/impl/testbench.go`
- [ ] Add `loadedLogs []BufferedLog` field
- [ ] In `loadLogsDir()`: store raw log before processing
- [ ] Support `logs.json` file at scenario root (in addition to `logs/` dir)

**0c. Add Log APIs to Testbench**
File: `comp/observer/impl/testbench_api.go`
- [ ] `GET /api/logs` - list all logs
- [ ] `GET /api/logs?start=X&end=Y` - logs in time window

**0d. Record Real Scenarios**
```bash
# Enable recording
cat >> dev/dist/datadog.yaml << EOF
observer:
  capture_metrics:
    enabled: true
  capture_logs:
    enabled: true
  parquet_output_dir: /tmp/observer-recording
  logs_output_file: /tmp/observer-recording/logs.json
EOF

# Run agent + workload
./bin/agent/agent run &
./demo-app --mode=memory-leak --duration=120s

# Copy to scenario
mkdir -p anomaly_datasets_converted/memory-leak-with-logs
cp /tmp/observer-recording/*.parquet anomaly_datasets_converted/memory-leak-with-logs/
cp /tmp/observer-recording/logs.json anomaly_datasets_converted/memory-leak-with-logs/
```

### Phase 1: Smart Log Buffer
- [ ] Add `LogBuffer` struct (pattern dedup + error logs)
- [ ] Hook into `ObserveLog()` to buffer
- [ ] Add method: `GetLogSummary() []LogPatternSummary`
- [ ] **Test**: Replay scenario, verify buffer captures expected patterns

### Phase 2: Health Score
- [ ] Add `HealthCalculator` struct
- [ ] Update on each anomaly
- [ ] Expose via API: `GET /api/health`
- [ ] **Test**: Replay scenario, verify health drops at expected times

### Phase 3: Context Packet + Auto-Flare
- [ ] Define `ContextPacket` struct
- [ ] Auto-generate on health drop > threshold
- [ ] Include: log patterns, error logs, correlations
- [ ] Write to disk
- [ ] Auto-trigger flare (configurable)
- [ ] **Test**: Replay scenario, verify context packet captures right data

### Phase 4: Flare Integration
- [ ] Add observer section to flare
- [ ] Include: health score, context packets, anomaly summary
- [ ] **Test**: Generate flare, verify observer data included

---

## Storage Format

**Decision**: JSON for logs (Option A)

```
scenario/
  metrics.parquet
  logs.json          # JSON lines
  events.json
```

Context packets also stored as JSON:
```
context_packets/
  2024-01-15T10-32-00.json
```

---

## Config

```yaml
observer:
  # Existing
  parquet_output_dir: /var/log/datadog/observer/metrics

  # New - Smart Log Buffer
  log_buffer_window_seconds: 60      # evict logs older than this
  log_buffer_max_patterns: 500       # max unique patterns to track
  log_buffer_max_errors: 200         # max error/warn logs to keep

  # New - Health Score
  health_score_window_seconds: 300   # time window for health calc

  # New - Context Packet + Auto-Flare
  context_packet_dir: /var/log/datadog/observer/context
  context_packet_retention: 24h
  context_packet_health_drop_threshold: 20   # generate on 20+ point drop
  auto_flare_on_health_drop: true            # trigger flare automatically
```

---

## Demo Scripts

### Phase 0: Record + Replay Scenario with Logs

```bash
# 1. Enable recording in config
cat >> dev/dist/datadog.yaml << 'EOF'
observer:
  capture_metrics:
    enabled: true
  capture_logs:
    enabled: true
  parquet_output_dir: /tmp/observer-recording
  logs_output_file: /tmp/observer-recording/logs.json
EOF

# 2. Build and run agent
dda inv agent.build --build-exclude=systemd
./bin/agent/agent run -c dev/dist/datadog.yaml &

# 3. Run workload that generates logs + metrics
./demo-app --mode=memory-leak --duration=120s

# 4. Stop agent, check recorded data
kill %1
ls -la /tmp/observer-recording/
# metrics_*.parquet, logs.json

# 5. Copy to scenario directory
mkdir -p comp/observer/anomaly_datasets_converted/memory-leak-recorded
cp /tmp/observer-recording/*.parquet comp/observer/anomaly_datasets_converted/memory-leak-recorded/
cp /tmp/observer-recording/logs.json comp/observer/anomaly_datasets_converted/memory-leak-recorded/

# 6. Start testbench and load scenario
./observer-testbench --scenarios-dir=comp/observer/anomaly_datasets_converted
curl -X POST localhost:8080/api/scenarios/memory-leak-recorded/load

# 7. Verify logs loaded
curl localhost:8080/api/logs | jq length

# 8. Get logs around anomaly time
curl "localhost:8080/api/logs?start=1704067200&end=1704067260" | jq
```

### Later: Full Health + Context Demo

```bash
# After phases 1-4 are built...

# 1. Replay scenario
./observer-testbench --scenarios-dir=scenarios
curl -X POST localhost:8080/api/scenarios/memory-leak-with-logs/load

# 2. Watch health score
curl localhost:8080/api/health
# {"score": 95, ...}
# ... as replay progresses ...
# {"score": 52, ...}

# 3. Check auto-generated context packet
curl localhost:8080/api/context-packets | jq
# Shows: trigger, blast radius, causal chain, log patterns, error logs

# 4. Verify in flare
./bin/agent/agent flare --include-observer-context
```

---

## Decisions Made

| Question | Decision |
|----------|----------|
| Log storage format | JSON (keep simple) |
| Memory footprint | Smart buffer: dedup by pattern, keep errors |
| Trigger | Auto-generate on health drop > threshold |
| PII/Privacy | Not handling for now |

## Open Questions

1. What's the right health drop threshold? (20 points? 30?)
2. Should auto-flare require user opt-in or be default on?
3. Rate limit on auto-flares? (don't spam during prolonged incident)
4. Should we also capture stack traces / goroutine dumps in context?

---

## Success Criteria

- [ ] Health score visible in testbench UI
- [ ] Context packet generated on injected failure
- [ ] Flare includes observer context
- [ ] Logs visible in context packet around anomaly time
- [ ] Can replay scenario with logs in testbench

---

## Immediate Next Steps (Phase 0) - Demo Generator Approach

### Task 1: Add LogWriter
File: `comp/observer/impl/log_writer.go` (new)
- JSON lines writer for logs
- Format: `{"timestamp":X,"content":"...","tags":[...],"level":"..."}`
- Method: `WriteLog(timestamp, content, tags, level)`

### Task 2: Add RecordingHandle
File: `comp/observer/impl/demo_recording.go` (new)
- Wraps observer.Handle to intercept ObserveMetric + ObserveLog
- Writes metrics to ParquetWriter, logs to LogWriter

### Task 3: Add --record flag to testbench
File: `cmd/observer-testbench/main.go`
- `--record --output <dir>` runs demo and records to files
- Creates: `<dir>/metrics.parquet`, `<dir>/logs.json`

### Task 4: Store raw logs in testbench
File: `comp/observer/impl/testbench.go`
- Add `loadedLogs []BufferedLog` field
- Modify `loadLogsDir()` to store raw logs
- Support `logs.json` at scenario root

### Task 5: Add log APIs to testbench
File: `comp/observer/impl/testbench_api.go`
- `GET /api/logs` - all logs
- `GET /api/logs?start=X&end=Y` - time window

### Task 6: Record demo scenario + test replay
```bash
./observer-testbench --record --output scenarios/demo-with-logs
./observer-testbench --scenarios-dir=scenarios
curl -X POST localhost:8080/api/scenarios/demo-with-logs/load
curl localhost:8080/api/logs | jq length
```
