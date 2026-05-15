# comp/anomalydetection/observer — AI Agent Guide

## What This Component Does

The observer is the engine at the heart of the anomaly-detection
pipeline. It ingests observations through Handles, stores them in an
in-memory columnar time-series store, runs detectors and correlators on
each advance cycle, and emits the result to subscribed reporters. See
`README.md` for the full pipeline diagram and extension guide.

## Architecture

### Two layers

| Layer | Code | Role |
|-------|------|------|
| **Component** (`observerImpl`) | `impl/observer.go` | Fx lifecycle, channel dispatch, Handle factory, HF check runners |
| **Engine** (`engine`) | `impl/engine.go` | Storage, detection, correlation, replay — the shared core |

The engine is a plain Go struct, not an Fx component. Both the live
observer and the testbench use the same engine.
