# demo-node: sample workload for localdog

`demo-node` is a small Express service. It sends **traces**, **logs** and **DogStatsD
metrics** to a Datadog Agent, and the Agent forwards them to the
[localdog](../../) server on `http://localhost:8282` instead of datadoghq.com.

```
demo-node ──traces :8126/tcp──▶ ┌───────────────┐
          ──statsd :8125/udp──▶ │ localdog-agent│ ──http──▶ localdog :8282
          ──app.log (file)────▶ │ (docker, host │
                                │  network)     │
                                └───────────────┘
```

## Quick start

```bash
cd test/localdog/demo
# 1. start localdog on :8282 (see test/localdog). The agent retries until it is up.
./run-agent.sh          # starts the agent container `localdog-agent`
./run-demo.sh           # starts demo-node in the background with the load generator on
```

To stop everything:

```bash
./stop-demo.sh
./stop-agent.sh
```

To run the app in the foreground:

```bash
cd node-app
npm install             # or: command npm install  /  scfw run npm install
LOG_FILE=/tmp/app.log npm start
```

## What it emits

### Traces (dd-trace, service `demo-node`, env `local`, version `1.0.0`)

| Route | What the trace contains |
|---|---|
| `GET /` | the express span only |
| `GET /api/users` | `cache.get` (service `demo-node-redis`). On a cache miss it adds `db.query` (service `demo-node-postgres`, SQL as the resource) and `cache.set` |
| `GET /api/users/:id` | the same cache and db spans. Ids above 25 return 404 |
| `POST /api/orders` | `db.query`, then an **outgoing HTTP call** to `GET /internal/inventory/:sku`. That gives an http client span and a server span in the same trace. Then `order.process` → `payment.authorize` (fails at `ERROR_RATE`) → 2× `db.query`. Returns 201, or 409 when stock is short |
| `GET /api/slow` | `report.generate` → `db.query`, 0.8–2.5 s |
| `GET /api/error` | throws a `TypeError` and returns 500, with error tags on the span |

The db queries also fail at random (`DbError`) at `ERROR_RATE/2`. Runtime metrics
(`runtime.node.*`) are on and go through DogStatsD.

### Logs (pino → `$LOG_FILE`, default `./logs/app.log`)

The log file is JSON lines with `level`/`status` (`debug|info|warn|error`), `message`,
`timestamp`, `service`, `env` and `host`. Logs injection adds a `dd` object to each line:
`dd.trace_id` (128-bit hex), `dd.span_id`, `dd.service`, `dd.env` and `dd.version`.
Lines also carry structured attributes: `http.method`, `http.url`, `http.route`,
`http.status_code`, `http.useragent`, `network.client.ip`, `usr.id`, `duration_ms`,
`error.kind`, `error.message`, `error.stack`, `order_id`, `sku`, and others.

The agent tails `$LOG_DIR/*.log` with `service: demo-node` and `source: nodejs`.

### DogStatsD metrics (hot-shots → `localhost:8125/udp`)

Every metric carries the global tags `env:local`, `service:demo-node` and `version:1.0.0`.

| Metric | Type | Tags |
|---|---|---|
| `demo.requests` | count | `route`, `method`, `status_code`, `status_class` |
| `demo.request.latency` | distribution (ms) | same |
| `demo.request.latency.histogram` | histogram (ms) | same |
| `demo.orders.created` | count | `sku` |
| `demo.orders.rejected` | count | `sku`, `reason` |
| `demo.orders.amount` | histogram | `sku` |
| `demo.errors` | count | `kind` |
| `demo.cache.requests` | count | `result:hit\|miss` |
| `demo.queue.depth` | gauge (every 5 s) | `queue:orders` |
| `demo.active_users` | gauge (every 5 s) | |
| `demo.unique_visitors` | set | |

## Configuration (env)

| Var | Default | |
|---|---|---|
| `PORT` | `3000` | HTTP port |
| `LOG_FILE` | `./logs/app.log` | JSON log file (`run-demo.sh` sets it to `$LOG_DIR/app.log`) |
| `LOADGEN` | `1` | `0` turns off the built-in load generator |
| `LOADGEN_RPS` | `4` | target requests per second |
| `ERROR_RATE` | `0.05` | probability of an injected failure |
| `DD_TRACE_AGENT_URL` | `http://localhost:8126` | |
| `DD_DOGSTATSD_HOST` / `DD_DOGSTATSD_PORT` | `localhost` / `8125` | |
| `LOG_LEVEL` | `debug` | |

Settings for the scripts (`run-agent.sh`, `run-demo.sh`):

| Var | Default |
|---|---|
| `LOCALDOG_URL` | `http://localhost:8282` |
| `LOG_DIR` | `/instance_storage/localdog-demo/logs` (`~/.localdog-demo/logs` when `/instance_storage` is missing) |
| `CONFD_DIR` | `<demo home>/conf.d` (the generated `demo-node.d/conf.yaml`) |
| `NETWORK_MODE` | `host`. Use `bridge` on older Docker Desktop for Mac: the agent then reaches localdog through `host.docker.internal` and publishes 8125/udp and 8126/tcp |
| `APM_TELEMETRY` | `false` (see the note below) |
| `AGENT_IMAGE` | `gcr.io/datadoghq/agent:7` |

### macOS

Docker Desktop 4.34 and later supports `--network host` once you turn on
*Settings → Resources → Network → Enable host networking*. Without it, use
`NETWORK_MODE=bridge ./run-agent.sh`. On macOS the script skips the `/proc` and
cgroup host mounts.

## Known caveats

- **Instrumentation telemetry is off.** The trace-agent's `/telemetry/proxy` always
  forwards over `https://` (see `pkg/trace/api/telemetry.go`), even when
  `apm_config.telemetry.dd_url` is `http://…`. A plain-http localdog therefore can't
  receive it. `run-agent.sh` sets `DD_APM_TELEMETRY_ENABLED=false` and `run-demo.sh` sets
  `DD_INSTRUMENTATION_TELEMETRY_ENABLED=false`.
- **Log trace ids are 128-bit.** dd-trace v6 writes `dd.trace_id` as 128-bit hex,
  for example `6aca7ddc0000000072e527714abca8d6`. The span payload carries the low
  64 bits as `trace_id` and the high 64 bits in the `_dd.p.tid` meta tag. To log
  64-bit decimal ids instead, run the app with
  `DD_TRACE_128_BIT_TRACEID_LOGGING_ENABLED=false`.
