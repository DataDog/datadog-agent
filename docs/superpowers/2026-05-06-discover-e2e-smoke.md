# End-to-end smoke test: krakend discover_config via rtloader bridge

This file captures how to validate the discover_config bridge end-to-end against
a real krakend container, using `dda inv discovery-dev.build-image` to
produce a self-contained dev image and `ddev env test` (from
integrations-core) to drive the e2e harness.

## What's verified end-to-end

1. Agent autodiscovery parses `auto_conf_discovery.yaml`.
2. AD reconciles the krakend template against the running krakend container.
3. The Go `discoverer` package marshals the service to JSON and crosses
   it into Python via the cgo `discover_config` C API to
   C++ `Three::discoverConfig`, which calls `AgentCheck.discover_config`.
4. `AgentCheck.discover_config` builds a `Service` dataclass, calls
   `KrakendCheck.generate_configs(service)`, and trial-runs each generated
   config with the real check implementation.
5. Krakend's OpenMetrics candidate for port 9090 succeeds.
6. The returned `list[dict]` JSON-roundtrips back to Go.
7. The agent schedules the krakend check with the resolved
   `openmetrics_endpoint`, and the check successfully scrapes metrics.

## Repos and branches involved

- `datadog-agent` branch `poc/config-discovery-c-agent` - agent-side AD,
  discoverer retry loop, C++/cgo `discover_config` bridge, and the local dev
  image task.
- `integrations-core` branch `poc/config-discovery-c-integrations` - Python
  `AgentCheck.discover_config`, OpenMetrics `generate_configs`, and the
  krakend discovery autoconf/e2e test.

Both repos must be at matching tip commits for the smoke test to work.

## Prerequisites

- Linux host, Docker daemon running, user has access to it.
- `dda` available (for `dda inv` tasks).
- `ddev` available (from integrations-core, used to drive the e2e).
- `krakend:2.10` pullable (`KRAKEND_VERSION` from
  `integrations-core/krakend/hatch.toml`).

## Build phase (host)

The agent binary's RUNPATH is hardcoded to `<repo>/dev/lib`. The
discovery-dev image mirrors that absolute path inside the container so
the dynamic linker resolves correctly without relinking. Three artifacts
need to be present in the source tree before building the image:

- `bin/agent/agent` - the agent binary with the `discoverer` package and
  `discover_config` cgo wrapper.
- `dev/lib/libdatadog-agent-{rtloader,three}.so*` - linked against the
  Python the container ships (3.13). The default cmake build links
  against the host's `python3.X-dev` package and produces a .so the
  container can't load; the bazel build avoids this.
- `dev/embedded/{lib,include}` - `libpython3.13.so.1.0` and supporting
  libs. The bazel-built rtloader has RPATH `<repo>/dev/embedded/lib`.

### 1. Build the agent binary

```bash
dda inv agent.build --build-exclude=systemd
```

Sanity check: `./bin/agent/agent version` should print the short SHA of
the latest commit on the branch.

### 2. Build the embedded Python + rtloader via Bazel

```bash
dda inv rtloader.install-with-bazel
```

This populates `dev/embedded/lib` with the Python 3.13-linked rtloader
and the matching libpython.

### 3. Restore the bazel-built rtloader in `dev/lib/`

`dda inv agent.build` runs cmake, which overwrites `dev/lib` with a
host-Python-linked rtloader. Copy the bazel artifacts back:

```bash
cp -P dev/embedded/lib/libdatadog-agent-rtloader* dev/lib/
cp -P dev/embedded/lib/libdatadog-agent-three.so dev/lib/
```

Verify it's the 3.13 build:

```bash
strings dev/lib/libdatadog-agent-three.so | grep -E "libpython3\.[0-9]+" | head -1
# libpython3.13.so.1.0
```

### 4. Build the discovery-dev image

```bash
dda inv discovery-dev.build-image
```

Produces `datadog/agent-dev:discovery-local`. The task fails fast if
`dev/lib`'s rtloader points at a libpython that isn't present in
`dev/embedded/lib` - the canonical "you forgot to restore the bazel
rtloader after agent.build" failure mode.

The Dockerfile (`test/dockerfiles/discovery-dev/Dockerfile`) layers the
agent binary and `dev/lib` + `dev/embedded` onto
`datadog/agent-dev:nightly-main-py3-jmx`, mirroring the host repo path
so RUNPATH/RPATH resolve. It also greps for the `discoverConfig` and
`discover_config` symbols so a missing-symbol regression fails the build,
not the e2e.

## Test phase

From integrations-core:

```bash
DDEV_E2E_AGENT=datadog/agent-dev:discovery-local \
DDEV_E2E_DOCKER_NO_PULL=1 \
  ddev env test --dev krakend py3.13-2.10 -- -k test_e2e_discovery
```

`DDEV_E2E_AGENT` points the harness at the local image; `DDEV_E2E_DOCKER_NO_PULL=1`
keeps it from pulling and overwriting it. `--dev` mounts the local
integration source so Python-side changes in `datadog_checks_base.utils.discovery`
or `krakend/datadog_checks/krakend` are picked up without rebuilding.

The test asserts metrics arrive from the discovered `openmetrics_endpoint` -
proof that the bridge round-tripped from Go to Python `discover_config`, to a
resolved config, to a scheduled check, to a successful scrape.

Stop the env when done:

```bash
ddev env stop krakend py3.13-2.10
```

## Negative scenarios worth automating later

The e2e covers the default-port happy path. Two more scenarios are valuable
smoke targets:

1. **Non-default port.** Edit `krakend.json` and the compose file to listen
   on a non-9090 port. On AD reconcile, OpenMetrics `generate_configs` should
   fall back to scanning the rest of the container's exposed ports and find it.

2. **Negative case.** Start a non-krakend container labelled with
   `com.datadoghq.ad.check_names='["krakend"]'` (e.g. `nginx:alpine`).
   Each candidate trial run should fail. No krakend check should be scheduled.

## Pitfalls

### `dda inv agent.build` silently overwrites the bazel rtloader

cmake links against the host's `python3.X-dev` and writes the result to
`dev/lib/`, replacing the bazel-built (Python 3.13-linked) .so files.
After every agent rebuild, restore them:

```bash
cp -P dev/embedded/lib/libdatadog-agent-rtloader* dev/lib/
cp -P dev/embedded/lib/libdatadog-agent-three.so dev/lib/
```

`discovery-dev.build-image` guards against this by checking that the
libpython the rtloader links against actually exists in `dev/embedded/lib`,
but the failure is still easy to introduce.

### `auto_conf_discovery.yaml` rejected with "no valid instances"

The file config provider rejects empty-instances templates unless
`discovery: {}` is also present. The yaml needs both `ad_identifiers:` and
`discovery: {}` for the discoverer path to take over.

### Python init timing

The discoverer triggers `InitPython` itself via the shared `pythonOnce`
when `python_lazy_loading` is true (default). The same idempotent
sync.Once is held by the python check loader, so multiple consumers race
safely. `Initializing rtloader` should appear exactly once per agent
process, ~6 s after start (when the first AD reconcile that matches a
discovery template fires).

## Late-arriving service: delayed-startup retry

The discovery trial-run retry validation uses a krakend container whose
entrypoint sleeps before exec'ing the actual binary, so the AD event
fires while the HTTP endpoint is still unreachable. The reproducer is
committed at `test/dockerfiles/discovery-dev/krakend-delayed/`:

- `docker-compose.yml` - the krakend service with the delayed entrypoint.
  Reads `${INTEGRATIONS_CORE_REPO}` from the environment to bind-mount
  the krakend test fixtures from the integrations-core checkout.
- `run_repro.sh` - orchestrates the run (starts agent, then the delayed
  krakend, watches logs, prints `agent configcheck` and `agent status`).
  By default it expects integrations-core at `../integrations-core`
  next to the agent repo; override with `INTEGRATIONS_CORE_REPO=/path`.

Run it after building `datadog/agent-dev:discovery-local`:

```
bash test/dockerfiles/discovery-dev/krakend-delayed/run_repro.sh
```

Expected sequence with the retry loop in place:

- t ~= 2 s: first `discover_config` trial run fails because the endpoint
  is not listening.
- t ~= 5-10 s: fast retry slots fire, still with no match.
- t ~= 10-60 s: 30 s retry slots fire periodically, still with no match.
- t ~= 60 s: krakend starts listening on :9090.
- Next retry tick after that (within 5 s): the generated OpenMetrics
  candidate succeeds, `discoveryRetryLoop` logs 1 schedule applied, and the
  krakend check goes [OK].

Expected log shape:

~~~
autodiscovery/discoverer: krakend.discover_config() failed for ...
10:17:44  autodiscovery: discovery retry tick applied 1 schedule(s), 0 unschedule(s)
~~~

`agent configcheck` after the match:

~~~
=== krakend check ===
Configuration source: file:/etc/datadog-agent/conf.d/krakend.d/auto_conf_discovery.yaml
openmetrics_endpoint: http://172.17.133.3:9090/metrics
~~~

`agent status` after the match:

~~~
krakend (1.4.1)
  Instance ID: krakend:d47601757ac15041 [OK]
  Total Runs: 2
  Metric Samples: Last Run: 84, Total: 168
~~~

discover_config was called 5 times total (1 initial + 4 retries); the 5th call succeeded
and `discoveryRetryLoop` applied the resulting ConfigChange.
