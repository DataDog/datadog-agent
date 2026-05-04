# observer-eval: Scrappy Training Data Collection via gs-flow

Runs gensim episodes with the Datadog Agent's observer + ScrappyCollector to capture
training data (JSONL metric snapshots + parquet recordings) for the Scrappy anomaly
detection model.

## Prerequisites

- **Local gs-flow** running at `localhost:8080` against `datadog-sandbox` cluster
  (see `~/dd/dd-source/domains/sims/apps/gs-flow/docs/run_books/LOCAL_DEVELOPMENT_GUIDE.md`)
- **gcloud auth**: `gcloud auth login` + `gcloud auth configure-docker us-east1-docker.pkg.dev`
- **Agent image** pushed to sandbox GAR (see [Building the Agent Image](#building-the-agent-image))
- **GENSIM_REPO_PATH** set to your gensim-episodes checkout

## Quick Start (single episode)

```bash
export GENSIM_REPO_PATH=~/dd/gensim-episodes
export DD_API_KEY=<your-key>
export DD_APP_KEY=<your-key>

# Build generator + episode service images, submit to local gs-flow
REGISTRY=us-east1-docker.pkg.dev/datadog-sandbox/gensim-images \
  ./build.sh --push --episodes "008_Slack_HAProxy_Outage:haproxy-state-sync-failure"

SANDBOX_GAR=us-east1-docker.pkg.dev/datadog-sandbox/gensim-images
curl -s -X POST http://localhost:8080/api/v1/jobs \
  -H 'Content-Type: application/json' \
  -d "{
    \"backend\": \"observer-eval\",
    \"generator_image\": \"$SANDBOX_GAR/observer-eval:latest\",
    \"secrets\": {
      \"AGENT_IMAGE\": \"$SANDBOX_GAR/agent-dev:scrappy-collect-amd64\",
      \"EPISODES\": \"008_Slack_HAProxy_Outage:haproxy-state-sync-failure\",
      \"GENSIM_MODE\": \"scrappy-collect\",
      \"LOGS_ENABLED\": \"false\",
      \"DD_API_KEY\": \"$DD_API_KEY\",
      \"DD_APP_KEY\": \"$DD_APP_KEY\",
      \"DD_SITE\": \"datadoghq.com\",
      \"GAR_REGISTRY\": \"$SANDBOX_GAR\"
    }
  }"
```

Poll status and collect artifacts:
```bash
JOB_ID=<from submission response>
curl -s http://localhost:8080/api/v1/jobs/$JOB_ID | jq .status

# Artifacts land on disk at:
ls /tmp/gs-flow-artifacts/jobs/$JOB_ID/artifacts/
```

## Building the Agent Image

The agent image must be linux/amd64 with the ScrappyCollector compiled in. Build from
an ARM Mac using cross-compilation in the dda dev container:

```bash
# 1. Start dev container (if not running)
dda env dev start

# 2. Install cross-compile toolchain (first time only)
dda env dev run -- bash -c 'apt-get update -qq && apt-get install -y -qq crossbuild-essential-amd64'

# 3. Build agent binary for linux/amd64
docker exec \
  -e CC=x86_64-linux-gnu-gcc \
  -e CXX=x86_64-linux-gnu-g++ \
  -e CGO_ENABLED=1 \
  -e GOARCH=amd64 \
  -e CGO_LDFLAGS="-L/root/repos/datadog-agent/dev/lib" \
  -e CGO_CFLAGS="-I/root/repos/datadog-agent/dev/include" \
  -e HOME=/root \
  dda-linux-container-default \
  bash -c 'cd /root/repos/datadog-agent && go build \
    -tags "docker containerd kubeapiserver kubelet orchestrator cri process secrets zlib zstd ec2 apm netcgo" \
    -ldflags "-w -s -r /opt/datadog-agent/embedded/lib" \
    -o bin/agent/agent-amd64 \
    ./cmd/agent'

# 4. Package and push Docker image
cat > /tmp/scrappy-agent.Dockerfile << 'EOF'
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y ca-certificates curl && apt-get clean && rm -rf /var/lib/apt/lists/*
COPY bin/agent/agent-amd64 /opt/datadog-agent/bin/agent/agent
COPY dev/lib/libdatadog-agent-rtloader.so.0.1.0 /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
COPY dev/lib/libdatadog-agent-rtloader.so /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so
COPY dev/lib/libdatadog-agent-three.so /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so
COPY cmd/agent/dist/ /etc/datadog-agent/
RUN mkdir -p /var/log/datadog /opt/datadog-agent/run /etc/datadog-agent/conf.d && chmod +x /opt/datadog-agent/bin/agent/agent
ENV PATH="/opt/datadog-agent/bin/agent:${PATH}" LD_LIBRARY_PATH="/opt/datadog-agent/embedded/lib" DD_CONF_PATH="/etc/datadog-agent"
ENTRYPOINT ["/opt/datadog-agent/bin/agent/agent"]
CMD ["run"]
EOF

GAR=us-east1-docker.pkg.dev/datadog-sandbox/gensim-images
docker buildx build --platform linux/amd64 -t "$GAR/agent-dev:scrappy-collect-amd64" \
  -f /tmp/scrappy-agent.Dockerfile . --push
```

Only rebuild when Go code in `comp/observer/` or `pkg/config/setup/config.go` changes.

## Output Format

Artifacts land in `/tmp/gs-flow-artifacts/jobs/<job-id>/artifacts/`:

| File | Description |
|------|-------------|
| `scrappy-collect.jsonl` | Scrappy training data (metric snapshots + log pattern context) |
| `results/<episode>/parquet/*.parquet` | Raw observer recordings (metrics + logs) |
| `results/<episode>/meta.json` | Episode metadata (duration, outcome, timestamps) |
| `observer-parquet.tar.gz` | Parquet tarball from background collector |

### JSONL line types

```jsonc
// Header (first line)
{"type": "header", "start_ts": "2026-04-29T...", "collector_version": "0.2"}

// Tick (one per detection cycle, ~every 16-30s)
{"data_time": 1777423845, "series": [{"ns": "system-checks-hf", "name": "system.mem.slab", "tags": [...], "points": [{"ts": 1777423845, "val": 123.95}]}]}

// Pattern context (periodic, maps hash → pattern text)
{"type": "patterns", "data_time": 1777423900, "patterns": [{"metric_name": "log.pattern.bd358c.count", "pattern": "Starting * on port *", "example": "Starting server on port 8080", "source": "log_metrics_extractor"}]}
```

## Data Organization

Collected data should be organized under `~/dd/scrappy/data/`:

```
~/dd/scrappy/data/
├── <episode-slug>/
│   ├── scrappy/scrappy-collect.jsonl
│   ├── parquet/*.parquet
│   └── meta/meta.json
```

## Known Limitations

- **Parquet collection is partial**: the background collector copies parquet every 60s
  but the vcluster may be recycled before the final copy. Scrappy JSONL is complete
  (flush-on-write), but parquet may only contain the last few snapshots.
- **Detection interval is ~16-30s**, not 1s. Depends on HF check runner initialization
  and available kubelet access in the vcluster.
- **Episode service images must be Python-based** for reliable cross-platform builds.
  Go-based episode services may fail to compile for linux/amd64 on ARM Macs.
