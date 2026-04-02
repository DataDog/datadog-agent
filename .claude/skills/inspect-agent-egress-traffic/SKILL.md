---
name: inspect-agent-egress-traffic
description: >
  Inspect what metrics/traces/logs the Datadog Agent is sending to intake by
  running a MiTM proxy (proxy-dumper) between the agent and Datadog endpoints.
  Use this skill when the user wants to verify intake isolation, debug metric
  cardinality, validate that specific metrics do/don't reach intake, or inspect
  agent egress traffic. Also trigger when the user mentions "proxy-dumper",
  "intake isolation", "what is the agent sending", "MiTM proxy", or wants to
  see what the forwarder is sending to Datadog.
---

# Inspect Agent Egress Traffic

Run a MiTM proxy between the Datadog Agent and Datadog intake endpoints to
inspect, filter, and validate outbound metric/trace/log payloads.

## Prerequisites

This skill requires the `proxy-dumper` Docker image, built from the
`datadog-agent-benchmarks` repository (Datadog-internal only).

Check if the user is a Datadog employee by testing for `ddtool` in PATH:

```bash
which ddtool >/dev/null 2>&1
```

If `ddtool` is not found, tell the user this skill requires internal Datadog
tooling and stop.

## Building proxy-dumper

The source lives at `~/dd/datadog-agent-benchmarks/docker/proxy-dumper/`
(or `$DATADOG_AGENT_BENCHMARKS_PATH/docker/proxy-dumper/`).

```bash
BENCHMARKS_PATH="${DATADOG_AGENT_BENCHMARKS_PATH:-$HOME/dd/datadog-agent-benchmarks}"
docker build -t proxy-dumper "$BENCHMARKS_PATH/docker/proxy-dumper/"
```

If the benchmarks repo isn't found, tell the user:
```
git clone git@github.com:DataDog/datadog-agent-benchmarks.git ~/dd/datadog-agent-benchmarks
```

Check if the image already exists before building — `docker image inspect
proxy-dumper >/dev/null 2>&1` skips the build if it's cached.

## proxy-dumper flags

| Flag | Default | Purpose |
|------|---------|---------|
| `-f/--prefix <string>` | `""` (all) | Filter metrics by name prefix (e.g. `-f system.` or `-f container.`) |
| `--points-print` | off | Include timestamps on each data point |
| `--print-origins` | off | Show metric origin metadata (MetricSource enum) |
| `--proxy-requests=false` | true | Capture only, don't forward to real intake (no API key needed) |
| `--proxy-requests=true` | (default) | Forward to real intake after inspection |
| `-l :PORT` | `:8081` | Listen address |
| `-p/--protobuf-print` | true | Print protobuf v2 series payloads |
| `-j/--json-print` | true | Print JSON v1 series payloads |
| `-k/--sketch-print` | true | Print DDSketch payloads |

## tmux orchestration

Use a dedicated tmux session called `egress-inspect` with two windows. These
commands are self-contained — no tmux skill dependency required.

### Setup

```bash
tmux new-session -d -s egress-inspect
tmux new-window -t egress-inspect -n proxy
tmux new-window -t egress-inspect -n agent
```

### Start proxy-dumper

```bash
tmux send-keys -t egress-inspect:proxy \
  "docker run --rm -p 8081:8081 proxy-dumper [FLAGS] 2>&1 | tee /tmp/proxy-dump.log" Enter
```

Replace `[FLAGS]` with the appropriate flags for the use case. Wait ~2 seconds
and verify it started:

```bash
sleep 2
tmux capture-pane -t egress-inspect:proxy -p | tail -5
# Should show "Starting server" log line
```

### Start the agent

Generate a temp config pointing ALL endpoints at the proxy. Note: `dd_url`
alone only routes metrics. Event platform pipelines (container lifecycle, logs,
container images) each need explicit `logs_dd_url` overrides due to a known
bug (see DataDog/datadog-agent#48814).

```bash
cat > /tmp/egress-inspect-datadog.yaml << 'YAML'
api_key: "0000001"
dd_url: "http://localhost:8081"
hostname: "egress-inspect"

# Event platform pipelines need explicit endpoint overrides.
# dd_url alone does not route these (bug: #48814).
container_lifecycle:
  enabled: true
  logs_dd_url: "http://localhost:8081"
container_image:
  logs_dd_url: "http://localhost:8081"
logs_config:
  logs_dd_url: "http://localhost:8081"
YAML
```

If `DDDEV_API_KEY` is set and the user wants real forwarding, use that instead
of the dummy key. Add any additional agent config the user specifies before
starting.

```bash
tmux send-keys -t egress-inspect:agent \
  "sudo ./bin/agent/agent run -c /tmp/egress-inspect-datadog.yaml 2>&1 | tee /tmp/agent-egress.log" Enter
```

### Monitor

```bash
# Check proxy is receiving traffic
tmux capture-pane -t egress-inspect:proxy -p | tail -20

# Check agent is running
tmux display-message -t egress-inspect:agent -p "#{pane_current_command}"

# Read full captured traffic
cat /tmp/proxy-dump.log

# Count occurrences of a specific metric
grep 'Metric="system.cpu.user"' /tmp/proxy-dump.log | wc -l
```

### Cleanup

```bash
tmux send-keys -t egress-inspect:agent C-c
sleep 1
tmux send-keys -t egress-inspect:proxy C-c
sleep 1
tmux kill-session -t egress-inspect
rm -f /tmp/egress-inspect-datadog.yaml
```

## Common use cases

### Verify HF metrics don't reach intake

```bash
# proxy-dumper filtering for system.* metrics, no forwarding
docker run --rm -p 8081:8081 proxy-dumper -f system. --proxy-requests=false
```

Run the agent for 60 seconds, then count:

```bash
grep 'Metric="system.cpu.user"' /tmp/proxy-dump.log | wc -l
```

At 15s check interval, expect ~4 occurrences per minute. If you see ~60, the
1s HF metrics are leaking through the forwarder.

### Inspect container metric cardinality

```bash
docker run --rm -p 8081:8081 proxy-dumper \
  -f container. --points-print --print-origins
```

Look for per-`container_id` tag explosion in the output.

### Full traffic inspection with forwarding

```bash
docker run --rm -p 8081:8081 proxy-dumper --proxy-requests=true
```

All traffic is visible AND forwarded to real Datadog intake. Requires a valid
`api_key` in the agent config.

### Count unique metric names being sent

```bash
grep 'Metric="' /tmp/proxy-dump.log \
  | sed 's/.*Metric="\([^"]*\)".*/\1/' \
  | sort -u | wc -l
```

### Measure per-metric submission rate

```bash
# After running for 60 seconds:
grep 'Metric="system.cpu.user"' /tmp/proxy-dump.log | wc -l
# Divide by 60 for per-second rate
```

### Check metric timestamps for cadence

```bash
grep -A1 'Metric="system.cpu.user"' /tmp/proxy-dump.log | grep Timestamp
# Look at the time gaps between consecutive entries
```
