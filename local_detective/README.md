# Local Detective

**Unsupervised Graph Auto-Encoder for Root Cause Detection**

Zero-shot anomaly detection using Graph Neural Networks. Learns from healthy data only, detects novel failures automatically.

## What Is This?

Local Detective identifies root causes of system failures by learning what "normal" looks like. During failures, it detects anomalies via reconstruction error - nodes behaving unusually stand out as root cause candidates.

**Key Innovation**: Trains only on healthy data, yet detects failure types it has never seen (zero-shot learning).

## Current Status

✅ **POC Complete - Model Trained on Real Data**
- Collected 89 healthy graph snapshots from OpenTelemetry demo workload
- Trained Graph Auto-Encoder: 92.3% loss reduction (0.052 → 0.004)
- Model: 12KB, 1,819 parameters, detection threshold: 0.007

⏳ **Next Phase**: Failure injection testing on real workloads

## Quick Start - Run the Full Experiment

Complete end-to-end workflow from scratch (takes ~15 minutes total).

### Prerequisites
- Kind Kubernetes cluster (`kind-gadget-dev`)
- Docker + kubectl + helm installed

### Step 1: Deploy OpenTelemetry Demo Workload

```bash
# Create namespace and deploy demo app
kubectl create namespace otel-demo --context kind-gadget-dev

helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm repo update
helm install otel-demo open-telemetry/opentelemetry-demo \
  --namespace otel-demo \
  --kube-context kind-gadget-dev \
  --set default.enabled=true

# Wait for pods to be ready (~2 minutes)
kubectl wait --for=condition=ready pod --all -n otel-demo --timeout=300s --context kind-gadget-dev
```

### Step 2: Build & Load Docker Image

```bash
cd ~/dd/datadog-agent/local_detective

docker build -t local-detective:latest -f deploy/Dockerfile .
kind load docker-image local-detective:latest --name gadget-dev
```

### Step 3: Generate Traffic

```bash
./scripts/generate_traffic.sh

# Verify traffic
kubectl logs -n otel-demo traffic-generator -f --context kind-gadget-dev
# Expected: "Request 1... Status: 200"
```

### Step 4: Collect Baseline (10 minutes)

```bash
kubectl apply -f deploy/baseline-collector.yaml --context kind-gadget-dev

# Monitor collection (60 samples @ 10s interval)
kubectl logs -n local-detective baseline-collector -f --context kind-gadget-dev
# Expected: "Collected graph with 774 nodes... [1/60]"
```

### Step 5: Train Model (2 minutes)

```bash
kubectl apply -f deploy/trainer.yaml --context kind-gadget-dev

# Monitor training
kubectl logs -n local-detective trainer -f --context kind-gadget-dev
# Expected: "Epoch 100: loss=0.004... Model saved"
```

### Step 6: Verify Results

```bash
# Copy model locally
kubectl cp local-detective/trainer:/app/models/trained_autoencoder.pt ./models/trained_autoencoder.pt --context kind-gadget-dev

# Visualize (optional)
python3 -m venv venv && source venv/bin/activate
pip install -r requirements.txt
python scripts/visualize_model_simple.py
```

### Step 7: Next - Failure Injection

Ready for Phase 4:
1. Inject failures (CPU spike, memory leak, network latency)
2. Collect failure graphs
3. Run detection to identify root cause nodes

## Project Structure

```
local_detective/
├── docs/
│   └── local detective.md         # Architecture & design doc
├── src/                           # Source code
│   ├── host_proc_collector.py     # /proc data collector
│   ├── graph_builder.py           # Graph construction
│   ├── gnn_model.py               # Auto-Encoder model
│   └── events.py                  # Event schema
├── scripts/                       # Executable scripts
│   ├── collect_baseline.py        # Baseline collection
│   ├── train_on_baseline.py       # Model training
│   ├── visualize_model_simple.py  # Model visualization
│   └── generate_traffic.sh        # Traffic generator
├── deploy/                        # Kubernetes deployments
│   ├── Dockerfile                 # Container image
│   ├── baseline-collector.yaml    # Collector pod
│   └── trainer.yaml               # Trainer pod
└── requirements.txt               # Python dependencies
```

## How It Works

1. **Data Collection**: Scrapes `/proc` for processes, network, PSI (system pressure)
2. **Training**: Unsupervised learning on healthy graphs (11D node features: CPU, memory, latency, etc.)
3. **Detection**: High reconstruction error = anomalous node = root cause

**Node Features (11D)**: node_type, cpu_pct, state, has_blocked, blocked_duration, latency, degree_centrality, betweenness, memory_rss, psi_combined, psi_critical

**Model**: Graph Auto-Encoder with 2-layer GCN (11D → 32D → 16D → 32D → 11D)

## Documentation

- **[README.md](README.md)** (this file) - Quickstart guide
- **[local detective.md](docs/local%20detective.md)** - Detailed architecture, design, and rationale

## Why Unsupervised?

| Supervised ML | Unsupervised (Ours) |
|---------------|---------------------|
| Needs labeled failures | Only needs healthy data |
| Fixed failure types | Detects novel failures |
| Requires incidents | Works from day one |

Zero-shot learning: Model never sees failures during training, yet detects them by recognizing "this doesn't look normal."

## Example Use Case

**Scenario**: Payment Service HTTP 504 timeouts

**Traditional debugging**: 30 minutes (check dashboards → network → database → SSH → logs)

**With Local Detective**: 5 seconds (capture graph → reconstruct → rank by error → "Root cause: PID 8472 locked /var/log/app.log")

## Requirements

- Python 3.12+, torch ≥2.0, torch-geometric ≥2.3
- Linux with /proc (or Kubernetes cluster)
- 512MB RAM for training, <100MB for inference

## Contact

- **Authors**: Valeri Pliskin, Matteo Bertrone
- **Team**: Q-Branch | **Slack**: #q-branch

---

**Status**: POC complete. Model trained on real data. Ready for failure injection testing.
