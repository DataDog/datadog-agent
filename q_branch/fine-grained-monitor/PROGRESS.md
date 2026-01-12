# Fine-Grained Monitor - Progress & Next Steps

## Current Status: Investigating Zero Metrics in Exports

**Branch:** `sopell/fine-grained-monitor-scenario-export`
**Last Commit:** `4d930771f55 fix(fgm): fix chart rendering in exported HTML`

---

## Active Investigation: Zero Metrics Issue

### Problem Statement

Scenario exports were showing all zeros for metric values. Logs showed `total_bytes=0` after file rotations, suggesting no data was being collected.

### Root Cause Analysis

**Finding: The collector IS working correctly.** Investigation revealed:

1. **Parquet files ARE being written with real data**
   - Direct inspection: `stat /data/.../metrics-*.parquet` shows 2.5 MB files
   - Consolidated files: 14-24 MB
   - Viewer API lists 190+ metrics being collected

2. **The `total_bytes=0` log was misleading**
   - File size was being read BEFORE rotation completes
   - At that point, the parquet writer hasn't flushed data to disk
   - The file appears empty even though data is buffered

3. **Metrics ARE being emitted correctly**
   - `gauge!()` calls route through CaptureRecorder
   - Registry receives metric registrations
   - StateMachine's `record_captures()` reads from Registry

### Code Investigation Path

Files examined during investigation:

| File | Finding |
|------|---------|
| `src/main.rs:430-490` | `total_bytes` read before rotation - root cause of misleading log |
| `src/observer/mod.rs` | `gauge!()` calls emit metrics correctly |
| `metrics-0.24.3/src/handles.rs` | `Gauge::noop()` has `inner: None`, drops `.set()` calls silently |
| `metrics-0.24.3/src/recorder/mod.rs` | Falls back to `NOOP_RECORDER` if no global recorder |
| `lading_capture/manager.rs` | `install()` sets global recorder, shares Registry via Arc |
| `lading_capture/state_machine.rs` | `record_captures()` reads from shared Registry |

### Fix Applied

**File:** `src/main.rs`

**Change:** Moved file size calculation from BEFORE rotation to AFTER rotation completes.

```rust
// BEFORE (incorrect - file not flushed yet):
let file_size = tokio::fs::metadata(&current_output_path).await...;
total_bytes += file_size;
// ... rotation happens ...

// AFTER (correct - file closed and flushed):
match response_rx.await {
    Ok(Ok(())) => {
        // Get file size AFTER rotation completes
        let file_size = tokio::fs::metadata(&current_output_path).await...;
        total_bytes += file_size;

        tracing::info!(
            rotation = rotation_count,
            file_size_bytes = file_size,  // NEW: shows actual size
            total_bytes = total_bytes,
            "File rotation completed"
        );
        // ...
    }
}
```

### Verification Status

- [x] Code compiles (`cargo check` passes)
- [ ] Deploy to cluster (blocked - Docker Desktop hung, machine reboot required)
- [ ] Verify logs show correct `file_size_bytes` after rotation
- [ ] Run scenario export and verify non-zero data

### Next Steps After Reboot

```bash
# 1. Deploy the fix
cd q_branch/fine-grained-monitor
./dev.py cluster deploy

# 2. Wait for at least one rotation (~90 seconds)
kubectl logs -n fine-grained-monitor -l app=fine-grained-monitor -c monitor --tail=50

# 3. Verify log shows non-zero file_size_bytes:
#    "File rotation completed { rotation: 1, file_size_bytes: 2535457, total_bytes: 2535457 }"

# 4. Test scenario export
./scenario.py run memory-leak
# wait ~2 min
./scenario.py export <run_id> -o /tmp/test-export.html
# Verify HTML contains non-zero metric values
```

---

## Completed Features: Scenario Export

### Export API Endpoint (`/api/export`)

- Query parameters: `namespace`, `labels`, `metrics`, `range`
- Returns filtered parquet with container metrics
- Memory protection: max 10M rows, max 100 containers
- ZSTD compression

### Export Command (`./scenario.py export`)

- Loads dashboard config from `scenarios/<name>/dashboard.json`
- Port-forwards to metrics-viewer in cluster
- Calls export API with dashboard filters
- Generates self-contained HTML with embedded parquet

### Self-Contained HTML Template

- All CSS inlined (light/dark themes)
- Parquet data embedded as base64
- uPlot loaded from CDN, works offline after cached
- Full chart rendering, container selection, zoom controls

---

## Files Changed in This Branch

| File | Description |
|------|-------------|
| `src/main.rs` | **FIX:** Read file size after rotation, not before |
| `scenario.py` | Added `export` command with HTML generation |
| `src/metrics_viewer/server.rs` | Added `/api/export` endpoint |
| `src/metrics_viewer/static/export-template.html` | Self-contained viewer template |
| `Dockerfile` | Updated to copy `scenarios/` directory |
| `scenarios/*/dashboard.json` | Moved from `dashboards/` |

---

## Known Issues

1. **Docker Desktop Hangs** - Occasionally becomes unresponsive during builds
   - Symptom: `docker build` never completes, `docker ps` hangs
   - Fix: Reboot machine (or restart Docker Desktop)
   - Possible cause: Disk pressure, memory exhaustion

2. **Label Selector Filtering** - Dashboard `label_selector` not yet used in export API

3. **CDN Dependencies** - uPlot requires network on first load

---

## Historical Context

### Metrics Flow (for reference)

```
observer::sample()
    │
    ├─► gauge!("cgroup.v2.memory.current", &labels).set(value)
    │       │
    │       └─► metrics crate with_recorder()
    │               │
    │               └─► CaptureRecorder.register_gauge()
    │                       │
    │                       └─► Registry.get_or_create_gauge()
    │
    └─► (60 samples accumulated per INTERVALS window)

StateMachine::record_captures()
    │
    ├─► Registry.gauges.iter() - reads all registered gauges
    │
    └─► Accumulator.accept() - stores in ring buffer
            │
            └─► flush() on rotation - writes to parquet
```

### Key Insight

The `metrics` crate uses a global recorder pattern. If `gauge!()` is called before `CaptureRecorder::install()`, the metrics go to `NoopRecorder` and are silently dropped. However, investigation confirmed the recorder IS installed before the observer loop starts - the issue was purely the misleading log message.
