# Local Detective - Unsupervised Root Cause Detection

**Authors**: Valeri Pliskin, Matteo Bertrone
**Status**: POC Complete - Model Trained on Real Data
**Last Updated**: 2025-12-30

## The Problem

When failures occur, engineers spend 20-40 minutes manually correlating logs, metrics, and traces to find root causes. A Payment Service times out with HTTP 504s—SREs see high network latency and investigate the network. The real culprit? A background process briefly locked a shared log file, causing the Payment service to block on a write. By the time anyone investigates, the lock is gone.

Traditional monitoring shows symptoms (high latency, error rates) but misses the local root cause buried in process relationships, file locks, or network dependencies.

## The Solution

**Local Detective** is an edge agent gadget that uses **unsupervised Graph Auto-Encoder** to automatically identify root causes. It models the local system as a graph (processes, files, network connections) and learns what "normal" looks like. During failures, it identifies anomalous nodes by reconstruction error—nodes behaving unusually are the root cause.

**Key Innovation**: Zero-shot learning. No labeled failure data required. The model only needs to observe healthy operation.

## How It Works

### Architecture

```
System Observation → Graph Builder → Auto-Encoder → Root Cause
     (proc fs)      (nodes+edges)   (reconstruct)   (high error)
```

**1. Graph Collection**
- Collect system state from `/proc`: processes, network connections, file operations
- Enrich with system pressure (PSI), memory usage, network latency
- Build temporal graph: nodes (processes/files/network) + edges (relationships)

**2. Training (Unsupervised)**
- Observe system during healthy operation (no failures, no labels)
- Auto-Encoder learns to compress and reconstruct normal graphs
- Model "memorizes" what healthy patterns look like

**3. Detection (Zero-Shot)**
- During failure: capture current system graph
- Try to reconstruct it using the trained model
- Per-node reconstruction error = anomaly score
- Nodes with high error = behaving abnormally = root cause candidates

### Node Features (11D)

Each node in the graph has 11 features:
```
• node_type (process/file/network/system)
• cpu_pct, state, has_blocked, blocked_duration
• latency, degree_centrality, betweenness
• memory_rss, psi_combined, psi_critical
```

System pressure (PSI) captures CPU/memory/I/O contention across the host.

### Model

**Graph Auto-Encoder** (1,819 parameters, 12KB)
```
Encoder: 11D → 32D → 16D (compress)
Decoder: 16D → 32D → 11D (reconstruct)
```

Training loss: MSE between input and reconstruction
Detection: `argmax(reconstruction_error)` = root cause

## What We Built

### POC Implementation

**Real Data Collection** (not synthetic):
- Deployed to Kind Kubernetes cluster
- Collected 89 healthy graph snapshots from OpenTelemetry demo workload
- 10-minute observation window with active HTTP traffic
- Graphs: 181-1,229 nodes per snapshot (average: 774 nodes)

**Training Results**:
- 100 epochs on real healthy graphs
- Loss reduction: 92.3% (0.052 → 0.004)
- Anomaly threshold: 0.007 (95th percentile)
- Model learns normal patterns: CPU ~12-20%, Memory ~0.7%, I/O ~5%

**Components**:
- `host_proc_collector.py` - Scrapes `/proc` for processes, network, PSI
- `graph_builder.py` - Builds temporal graph with rolling window
- `gnn_model.py` - Graph Auto-Encoder with reconstruction-based detection
- Kubernetes deployment for baseline collection and training

### Current Status

✓ **Phase 1**: Real data collection from Kubernetes cluster
✓ **Phase 2**: Trained model on 89 healthy graphs
✓ **Phase 3**: Model ready for failure testing
⏳ **Phase 4**: Validate detection on injected failures
⏳ **Phase 5**: Deploy as DaemonSet for continuous monitoring

## Why This Approach Works

**Unsupervised vs Supervised**:

| Supervised ML | Unsupervised (Our Approach) |
|---------------|---------------------------|
| Needs labeled failure examples | Only needs healthy operation |
| Fixed failure scenarios | Detects novel failures |
| Requires incident data | Works from day one |
| Static model | Continuous learning |

**Key Advantages**:
- **Zero labels**: No manual annotation of failures
- **Novel failures**: Detects unknown failure types
- **Adaptive**: Learns customer-specific patterns
- **Lightweight**: 12KB model, <1s inference, <100MB memory

## Example Scenario

**Failure**: Payment Service HTTP 504 timeouts

**Traditional Approach**:
1. Check dashboards → See high latency
2. Investigate network → Nothing wrong
3. Check database → Queries fast
4. SSH to host → Inspect processes
5. Review logs → Find file lock after the fact
6. **Total time: 30 minutes**

**With Local Detective**:
1. Anomaly detected (error rate spike)
2. Capture system graph
3. Reconstruct via Auto-Encoder
4. Rank nodes by reconstruction error
5. Report: "Root cause: Process PID 8472 (log-rotate) locked /var/log/app.log, blocked Payment Service writes"
6. **Total time: 5 seconds**

## Next Steps

1. **Failure Injection Testing**
   - CPU spike, memory leak, network latency, file locks
   - Measure Top-1, Top-3, Top-5 accuracy
   - Tune anomaly threshold

2. **DaemonSet Deployment**
   - Continuous graph collection
   - Real-time anomaly detection
   - Integration with alerting systems

3. **Production Integration**
   - Replace `/proc` scraping with eBPF (via system-probe)
   - Connect to existing USM/NPM data streams
   - Output to Datadog Events API

4. **Continuous Learning**
   - Adapt model to customer workloads
   - Online learning during operation
   - Periodic model updates

## Files

```
local_detective/
├── src/
│   ├── events.py              # Event schema (no labels!)
│   ├── host_proc_collector.py # Real /proc data collection
│   ├── graph_builder.py       # Graph construction
│   └── gnn_model.py           # Auto-Encoder + detector
├── scripts/
│   ├── collect_baseline.py    # Baseline collection
│   ├── train_on_baseline.py   # Training on real data
│   └── generate_traffic.sh    # Traffic generation
├── models/
│   ├── trained_autoencoder.pt # Trained model (12KB)
│   └── healthy_baseline/      # 89 graphs (~5MB)
└── docs/
    ├── PLAN.md                # This file
    ├── MODEL_SUMMARY.md       # Training results
    └── DATA_COLLECTION_SUMMARY.md
```

## References

- Graph Auto-Encoder: https://arxiv.org/abs/1611.07308
- PyTorch Geometric: https://pytorch-geometric.readthedocs.io/
- Linux PSI: https://www.kernel.org/doc/html/latest/accounting/psi.html

---

**The Innovation**: Machine learning that doesn't need labeled data. The agent learns your "normal" and automatically detects root causes when things break.
