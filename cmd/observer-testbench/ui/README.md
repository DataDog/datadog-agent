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
│ (CUSUM, LightESD)    │         │ Inspect anomalies    │
│         ↓            │         │ Click correlations   │
│ Detect correlations  │         └──────────────────────┘
└──────────────────────┘
```

**Key principle**: Deterministic. Same data + same code = same results. The observer acts as a queryable database—load a scenario, components compute results, UI displays them.

## Project Layout

```
cmd/observer-testbench/
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

comp/observer/impl/
├── testbench.go         # Scenario loading, analysis orchestration
├── testbench_api.go     # REST API handlers
├── metrics_detector_cusum.go # CUSUM change-point detector
└── storage.go           # Time series storage
```

## Running

**Terminal 1** — Go server (auto-rebuild on changes):
```bash
cd cmd/observer-testbench
watchexec -r -e go -- go run . -scenarios-dir=../../scenarios
```

**Terminal 2** — UI dev server:
```bash
cd cmd/observer-testbench/ui
npm install
npm run dev
```

Open http://localhost:5173

## UI Controls

- **Drag** on chart to zoom time range
- **Middle-drag** to pan when zoomed
- **Shift+click** correlations to multi-select
- **Resize** sidebar by dragging right edge
- Tree buttons: Expand / Collapse / Focus (hide unselected)
