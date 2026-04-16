# PAR Dual-Process — POC

**Problem:** PAR runs as a single Go process consuming ~60–70 MB RSS at idle on every node, even when no actions are executing.

**Solution:** Split into two binaries. A tiny Rust control plane runs continuously; the Go executor is spawned on demand and self-terminates when idle.

```
par-control (Rust, ~1-2 MB RSS, always-on)
  │  reads datadog.yaml
  │  polls OPMS for action tasks
  │  manages executor lifecycle
  │
  └─── fork+exec ──→ par-executor (Go, spawned on demand)
         │  same code as today's PAR, minus the OPMS polling loop
         │  HTTP/1.1 over Unix socket (/tmp/par-executor.sock)
         │  self-terminates after idle timeout
         └─────────────────────────────────────────────────────
```

## What's in this branch

| Path | Description |
|------|-------------|
| `cmd/par-executor/` | Go executor binary entry point |
| `comp/privateactionrunner/executor/` | FX component — stripped PAR minus OPMS loop |
| `controlplane/rust/` | Rust control plane (par-control) |
| `Dockerfiles/par-dual/` | Standalone Dockerfile + docker-compose for local testing |

## par-control (Rust)

Reads config directly from `datadog.yaml` — no Go wrapper needed. Follows the same code conventions as `system-probe-lite` (`pkg/discovery/module/rust/`): same Clippy lints, signal handling, capability drop, `dd-agent-log`.

```
controlplane/rust/src/
  main.rs            — startup, signals, OPMS polling loop
  cli.rs             — arg parsing (--config, --executor-*, --log-*)
  par_config.rs      — reads PAR section of datadog.yaml (URN, private key, intervals)
  jwt.rs             — ES256 JWT for OPMS auth (p256 crate, mirrors util/jwt.go)
  opms.rs            — DequeueTask / PublishSuccess / PublishFailure / Heartbeat
  executor.rs        — IDLE→RUNNING state machine, process liveness, watchdog
  executor_client.rs — HTTP/1.1 over UDS client (hyper + UnixStream)
```

Build (Linux, via Bazel):
```bash
bazelisk build //pkg/privateactionrunner/controlplane/rust:par-control
```

## par-executor (Go)

The existing PAR binary with the control-plane responsibilities removed:

- **Removed:** OPMS polling loop, self-enrollment, CommonRunner health checks
- **Kept:** FX stack, Remote Config key subscription, task signature verification, credential resolution, action allowlist, all 34+ bundles, metrics

The UDS HTTP server replaces `WorkflowRunner.Start()` as the task intake point. Concurrency is bounded by `private_action_runner.task_concurrency` (default 5), matching today's behaviour.

Build:
```bash
go build -o bin/par-executor/par-executor ./cmd/par-executor/
# or via invoke:
dda inv par-executor.build
```

## IPC protocol

HTTP/1.1 over Unix Domain Socket — same pattern as system-probe ↔ core-agent.

```
POST /execute   {"raw_task": "<base64 OPMS bytes>", "timeout_seconds": N}
                → {"output": {...}, "error_code": 0}

GET  /debug/ready   — polled by par-control during STARTING state
GET  /debug/health  — watchdog ping (30 s interval, 3-strike kill)
```

> **Note for production:** for payloads approaching the 15 MB action output limit, replace JSON+base64 with binary length-framing over the same UDS. See discussion in the PR.

## Local Docker test

```bash
# Build
docker build -f Dockerfiles/par-dual/Dockerfile -t par-dual:local .

# Run with fakeintake as mock OPMS
mkdir -p /tmp/par-test-config
cat > /tmp/par-test-config/datadog.yaml <<EOF
private_action_runner:
  enabled: true
  actions_allowlist:
    - com.datadoghq.remoteaction.testConnection
    - com.datadoghq.remoteaction.rshell.runCommand
EOF

DD_CONFIG_DIR=/tmp/par-test-config \
  docker compose -f Dockerfiles/par-dual/docker-compose.yml up
```

## Tests

```bash
# IPC protocol tests (macOS + Linux)
go test -tags test -run TestIPC ./comp/privateactionrunner/executor/impl/

# Subprocess e2e tests (Linux only — builds and runs par-executor binary)
go test -tags test -run TestE2E ./comp/privateactionrunner/executor/impl/

# Rust unit tests
cargo test -p par-control
```

## K8s deployment (pending)

PAR runs as a sidecar in the agent DaemonSet, managed by the datadog-operator. The operator change needed is a single command swap in `privateActionRunnerContainer()` in `datadog-operator/internal/controller/datadogagent/component/agent/default.go`:

```go
// from:
"/opt/datadog-agent/embedded/bin/privateactionrunner", "run", "-c=...", "-E=..."
// to:
"/opt/datadog-agent/embedded/bin/par-control", "run",
  "--config", "/etc/datadog-agent/datadog.yaml",
  "--executor-cfgpath",  "/etc/datadog-agent/datadog.yaml",
  "--executor-extracfg", "/etc/datadog-agent/privateactionrunner.yaml",
```

All volumes, RBAC, capabilities, and ConfigMaps remain unchanged.
