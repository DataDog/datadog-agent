# localdog

localdog is a local Datadog backend for the Datadog Agent. Point an Agent running on your
laptop at it and explore the metrics, logs and traces it sends in a web app that uses the same
components as the Datadog Log Explorer, Trace Explorer, Metrics Explorer and dashboards.
Nothing leaves your machine.

```
 your app ──traces/logs/dogstatsd──▶ Datadog Agent ──intake payloads──▶ localdog :8282 ◀── web app
```

- **Intake**: localdog accepts what the Agent sends to Datadog: `/api/v2/series`,
  `/api/v1/series`, `/api/beta/sketches`, `/api/v1/check_run` (metrics), `/api/v2/logs`
  (logs), and `/api/v0.2/traces` (traces, including the indexed v1 payload format). Payloads are
  decoded with the fakeintake parsers ([../fakeintake](../fakeintake)). Every other route
  (metadata, processes, APM stats...) is accepted and dropped.
- **Query API**: localdog implements the part of the Datadog UI API that the product components
  call, on top of the stored data: `/api/v1/logs-analytics/{list,aggregate,facet_info,fetch_one}`
  for `type=logs` and `type=trace`, `/api/ui/event-platform/<track>/facets`,
  `/api/ui/trace/<trace_id>`, and `/api/ui/query/{timeseries,scalar}` (metric queries such as
  `sum:demo.requests{env:local} by {route}.as_count()`, with formulas). It answers CORS preflights,
  including Chrome's Private Network Access, so a page hosted elsewhere can read from localhost.
- **Web app**: `static-apps/localdog` in web-ui. Like lapdog, it rewrites the Datadog API calls of
  the product components to localdog. Either open the hosted app, or let localdog serve a build of
  it at `http://localhost:8282/`.
- **Storage**: in memory, with limits (`-max-logs`, `-max-spans`, `-max-points`, `-retention`),
  snapshotted to `~/.localdog` every minute and on shutdown (`-data-dir ""` disables persistence).

## Quick start

From the datadog-agent repository:

```bash
dda inv localdog.run      # build and start localdog on http://127.0.0.1:8282
dda inv localdog.agent    # start a Datadog Agent container that sends everything to localdog
dda inv localdog.demo     # start an instrumented Node.js app that generates traffic
```

Then open http://localhost:8282/ (when the web app is built, see below), and stop the demo with
`dda inv localdog.stop`.

### Use your own agent

Any Agent works, with any API key, as long as its intakes point at localdog. With environment
variables:

```bash
DD_API_KEY=00000000000000000000000000000000
DD_DD_URL=http://localhost:8282
DD_APM_DD_URL=http://localhost:8282
DD_PROCESS_CONFIG_PROCESS_DD_URL=http://localhost:8282
DD_LOGS_ENABLED=true
DD_LOGS_CONFIG_LOGS_DD_URL=localhost:8282
DD_LOGS_CONFIG_LOGS_NO_SSL=true
DD_LOGS_CONFIG_FORCE_USE_HTTP=true
DD_APM_TELEMETRY_ENABLED=false          # the telemetry proxy always uses https
DD_REMOTE_CONFIGURATION_ENABLED=false
```

or in `datadog.yaml`:

```yaml
api_key: 00000000000000000000000000000000
dd_url: http://localhost:8282
apm_config:
  apm_dd_url: http://localhost:8282
  telemetry:
    enabled: false
logs_enabled: true
logs_config:
  logs_dd_url: localhost:8282
  logs_no_ssl: true
  force_use_http: true
process_config:
  process_dd_url: http://localhost:8282
remote_configuration:
  enabled: false
```

An Agent in a container reaches localdog through `--network host` (Linux, or Docker Desktop with
host networking enabled), or `host.docker.internal:8282` otherwise. See [demo/](demo/) for a
complete container setup that also redirects the less common forwarders.

### The web app

The web app lives in web-ui (`static-apps/localdog`). With a web-ui checkout next to
datadog-agent:

```bash
dda inv localdog.build-ui              # yarn build of static-apps/localdog
dda inv localdog.run                   # serves the build at http://localhost:8282/
dda inv localdog.build --with-ui       # or: a single binary with the web app embedded
./test/localdog/build/localdog
```

During web app development, run `yarn dev` in web-ui and open
https://localhost:8443/static-apps/localdog/ — the app talks to `http://localhost:8282` (override
with `?localdog=http://host:port`).

## Development

```bash
dda inv localdog.build   # test/localdog/build/localdog
dda inv localdog.test
```

- `store/`: in-memory store, Datadog search query parser (`service:web -status:info
  @http.status_code:>=500 "timeout"`), metric query engine, snapshots.
- `server/intake.go`: Agent intake routes.
- `server/api_*.go`: Datadog UI API subset used by the web app.
- `cmd/localdog`: the binary.
- `demo/`: Agent container and demo Node.js app.

Useful endpoints: `GET /info` and `GET /api/localdog/stats` (what was received),
`POST /api/localdog/reset` (drop everything), `GET /api/localdog/metrics`.
