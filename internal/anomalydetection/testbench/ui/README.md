# Observer Test Bench

Interactive UI for iterating on observer anomaly detection components.

## Architecture

```
Go Server (:8080)                 React UI (:5173)
┌──────────────────────┐         ┌──────────────────────┐
│ Load scenario        │         │ Select scenario      │
│ (parquet/logs/demo)  │  HTTP   │ Toggle detectors     │
│         ↓            │◄───────►│ Browse series tree   │
│ Run detectors        │  JSON   │ View charts + zoom   │
│ (BOCPD, scans)       │         │ Inspect anomalies    │
│         ↓            │         │ Click correlations   │
│ Detect correlations  │         └──────────────────────┘
└──────────────────────┘
```

**Key principle**: Deterministic. Same data + same code = same results. The observer acts as a queryable database—load a scenario, components compute results, UI displays them.

## Project Layout

```
internal/anomalydetection/testbench/
├── main.go              # Entry point, starts HTTP server
└── ui/
    ├── src/
    │   ├── App.tsx      # Main app, state management
    │   ├── api/client.ts
    │   ├── components/
    │   │   ├── SeriesTree.tsx          # Collapsible series selector
    │   │   ├── MetricsChart.tsx     # D3 chart with brush zoom
    │   │   └── ChartWithAnomalyDetails.tsx
    │   └── hooks/useObserver.ts        # API polling, reconnect
    └── package.json

internal/anomalydetection/testbench/bench/
├── bench.go             # Scenario loading and analysis orchestration
├── api.go               # REST API handlers
└── output.go            # Headless evaluation output
```

## Running

**Terminal 1** — Go server (auto-rebuild on changes):
```bash
cd internal/anomalydetection/testbench
watchexec -r -e go -- go run . -scenarios-dir=../../scenarios
```

**Terminal 2** — UI dev server:
```bash
cd internal/anomalydetection/testbench/ui
npm install
npm run dev
```

Open `http://localhost:5173` in your browser.

## UI Controls

- Loading a scenario selects up to 50 recorded metric groups with the most
  detections from enabled detectors. Ties are sorted by metric name; telemetry
  and log-derived virtual groups are excluded from this default selection.
- **Drag** on chart to zoom time range
- **Middle-drag** to pan when zoomed
- **Shift+click** correlations to multi-select
- **Resize** sidebar by dragging right edge
- **Anomaly count** below each metric expands or collapses its anomaly list.
  Lists start collapsed; clicking a chart marker opens the matching details.
- Tree buttons: Expand / Collapse / Focus (hide unselected)
