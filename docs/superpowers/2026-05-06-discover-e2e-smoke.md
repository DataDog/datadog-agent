# End-to-end smoke test: krakend discover() via rtloader bridge

This file captures the exact sequence that successfully validates the Plan B
implementation (Go discoverer + rtloader runDiscover + Python `discover()`
classmethod) end-to-end against a real krakend container.

The current procedure is manual. It exists to be the basis for an automated
test plan; the steps and pitfalls below are the load-bearing facts an
automated harness needs to honor.

## What's verified end-to-end

1. Agent autodiscovery parses `auto_conf_discovery.yaml`.
2. AD reconciles the krakend template against the running krakend container.
3. The new Go `discoverer` package marshals the service to JSON and crosses
   it into Python via the cgo `run_discover` C API → C++ `Three::runDiscover`
   → `datadog_checks.base.utils.discovery._run_discover` Python helper.
4. The helper builds a `Service` dataclass, calls
   `KrakendCheck.discover(service)`.
5. Krakend's `discover()` runs an HTTP probe (`http_probe` +
   `is_prometheus_exposition`) against the container's port 9090.
6. The returned `list[dict]` JSON-roundtrips back to Go.
7. The agent schedules the krakend check with the resolved
   `openmetrics_endpoint`, and the check successfully scrapes metrics.

## Repos and branches involved

- `datadog-agent` branch `vitkyrka/advanced-autoconfig-krakend` — agent-side
  Go + C++ + cgo bridge.
- `integrations-core` branch `vitkyrka/disco-autoconfig` — Plan A (Python
  helpers in `datadog_checks_base.utils.discovery`), Plan B Task 4
  (`_run_discover` bridge helper), and the krakend `discover()` migration.

Both repos must be at the matching tip commits for the smoke test to work.

## Prerequisites

- Linux host (this procedure was run on aarch64 Ubuntu 24.04).
- Docker daemon running and the user has access to it.
- A `~/api` file that exports `DD_API_KEY` / `DD_SITE` / etc. (sourced by
  `docker-agent-run.sh`). Real API key not strictly required — the agent
  will accept any non-empty value, payloads will fail to upload but the
  check will still run.
- Existing krakend image accessible via `docker pull krakend:2.10` (the
  `KRAKEND_VERSION` matrix value from `integrations-core/krakend/hatch.toml`).
- `datadog/agent-dev:nightly-main-py3-jmx` image pulled. As of the run that
  produced this file, that nightly is built against Python 3.13.
- The helper script
  `/home/vagrant/go/src/github.com/DataDog/experimental/users/vincent.whitchurch/hacks/bin/docker-agent-run.sh`
  on PATH or invoked by absolute path. (Sources `~/api`, runs the agent
  with the standard set of host bind mounts: `/var/run/docker.sock`,
  `/proc`, `/sys/fs/cgroup`, etc.)

## Build phase (host)

The agent binary's RUNPATH is hardcoded to the host's
`<repo>/dev/lib` directory. Inside the container we bind-mount that path
read-only at the same absolute location so the dynamic linker resolves
correctly. The bind-mount strategy hinges on this fact.

The agent binary itself, the rtloader shared libraries, and the Bazel-managed
embedded Python (`dev/embedded`) all need to be built locally. The
container's stock copies are not sufficient because:

- the agent binary needs the new `discoverer` package and the cgo
  `run_discover` symbol;
- `libdatadog-agent-rtloader.so` and `libdatadog-agent-three.so` need to
  carry the new `runDiscover` virtual method, the new `run_discover` C
  API, and be linked against the same Python ABI as the container.

### 1. Build the agent binary

```bash
cd /home/vagrant/go/src/github.com/DataDog/datadog-agent
dda inv agent.build --build-exclude=systemd
```

Sanity check:

```bash
./bin/agent/agent version
# Agent <version>-devel - Meta: git.NN.<sha> - ...
```

The binary lands at `bin/agent/agent`. The version line must include the
short SHA of the latest commit on the branch (proves the build incorporated
your local Plan B Go changes).

### 2. Build the embedded Python + rtloader via Bazel

Important: the bazel build is what produces a *Python-3.13-linked*
`libdatadog-agent-three.so`. The default cmake build (`dda inv rtloader.make`)
links against the host's `python3.12-dev` package, which causes
`libpython3.12.so.1.0: cannot open shared object file` at agent runtime
inside the 3.13 container. The bazel build avoids this — it brings its own
Python 3.13 toolchain.

```bash
dda inv rtloader.install-with-bazel
```

This populates:

- `dev/embedded/lib/libpython3.13.so.1.0` (the Python runtime the rtloader
  will dlopen).
- `dev/embedded/lib/libdatadog-agent-rtloader.so.0.1.0` and
  `libdatadog-agent-three.so` (linked against Python 3.13).
- `dev/embedded/include/python3.13/...` (headers, not used at runtime).
- A pile of supporting libs (libcrypto, libssl, libsqlite3, libffi, …).

### 3. Replace the cmake-built rtloader in `dev/lib/`

The agent binary's RUNPATH points at `dev/lib`, not `dev/embedded/lib`. Copy
the bazel-built artifacts so the agent finds the 3.13-linked rtloader:

```bash
cp -P dev/embedded/lib/libdatadog-agent-rtloader* dev/lib/
cp -P dev/embedded/lib/libdatadog-agent-three.so dev/lib/
```

Sanity-check that the new rtloader contains the runDiscover symbol:

```bash
nm dev/lib/libdatadog-agent-three.so | grep -i discover
# should print: _ZN5Three11runDiscoverEP16RtLoaderPyObjectPKc
nm dev/lib/libdatadog-agent-rtloader.so | grep -i discover
# should print: run_discover
```

And that it's linked against 3.13:

```bash
strings dev/lib/libdatadog-agent-three.so | grep -E "libpython3\.[0-9]+|/python3\.[0-9]+/" | head -3
# should reference python3.13, not python3.12
```

## Test phase

### 4. Bring up the krakend dev environment

```bash
cd /home/vagrant/go/src/github.com/DataDog/integrations-core/krakend/tests/docker
KRAKEND_VERSION=2.10 docker compose up -d
```

Wait for healthy:

```bash
until [ "$(KRAKEND_VERSION=2.10 docker compose ps --format json | jq -r 'map(select(.Health=="healthy")) | length')" = "2" ]; do sleep 2; done
```

(or just sleep ~15s — both containers start in <20s).

Confirm the OpenMetrics endpoint is live:

```bash
curl -s -o /dev/null -w "%{http_code} %{content_type}\n" http://localhost:9090/metrics
# 200 text/plain; version=0.0.4; ...
```

### 5. Run the agent with bind-mounts

```bash
docker rm -f dd-agent-foo 2>/dev/null
/home/vagrant/go/src/github.com/DataDog/experimental/users/vincent.whitchurch/hacks/bin/docker-agent-run.sh \
  -v "/home/vagrant/go/src/github.com/DataDog/integrations-core/krakend/datadog_checks/krakend:/opt/datadog-agent/embedded/lib/python3.13/site-packages/datadog_checks/krakend" \
  -v "/home/vagrant/go/src/github.com/DataDog/integrations-core/datadog_checks_base/datadog_checks/base/utils/discovery:/opt/datadog-agent/embedded/lib/python3.13/site-packages/datadog_checks/base/utils/discovery" \
  -v "/home/vagrant/go/src/github.com/DataDog/integrations-core/krakend/datadog_checks/krakend/data/auto_conf_discovery.yaml:/etc/datadog-agent/conf.d/krakend.d/auto_conf_discovery.yaml:ro" \
  -v "/home/vagrant/go/src/github.com/DataDog/datadog-agent/bin/agent/agent:/opt/datadog-agent/bin/agent/agent" \
  -v "/home/vagrant/go/src/github.com/DataDog/datadog-agent/dev/lib:/home/vagrant/go/src/github.com/DataDog/datadog-agent/dev/lib:ro" \
  -v "/home/vagrant/go/src/github.com/DataDog/datadog-agent/dev/embedded:/home/vagrant/go/src/github.com/DataDog/datadog-agent/dev/embedded:ro" \
  -d datadog/agent-dev:nightly-main-py3-jmx
```

The six bind mounts, in order:

1. **krakend integration source** — overlays the container's stock krakend
   package with the local one (which has the new `discover()` classmethod).
2. **`datadog_checks_base.utils.discovery`** — overlays the container's
   stock helper module with the local one (Plan A helpers + Task 4
   `_bridge.py`). Both `Service`/`Port` types and `_run_discover` come
   from here.
3. **`auto_conf_discovery.yaml` into `conf.d`** — agent reads autoconfig
   files from `/etc/datadog-agent/conf.d/<integration>.d/`, NOT from the
   integration package's `data/` directory. This is the most commonly
   missed bind in this setup.
4. **agent binary** — the built agent with the new `discoverer` package
   wiring.
5. **`dev/lib`** — host RUNPATH of the agent binary; provides the rebuilt
   rtloader + three .so files.
6. **`dev/embedded`** — RPATH of the bazel-built rtloader; provides
   `libpython3.13.so.1.0` and supporting .so files.

The agent must also be on the same docker network as the krakend container
to reach it. The compose file creates `docker_default` (or whatever the
parent dir name resolves to). Connect after start:

```bash
docker network connect docker_default dd-agent-foo
```

(If `docker_default` is busy, the agent's existing connection persists; the
error "endpoint with name dd-agent-foo already exists in network
docker_default" is benign.)

### 6. Wait for the discoverer to lazy-init Python and probe

No manual intervention needed. The discoverer's bridge mirrors the
python check loader convention: when `python_lazy_loading` is true
(default), the first call to `RunDiscover` triggers `InitPython` via
the shared `pythonOnce` sync.Once. Python comes up, the probe runs,
the check is scheduled.

Typical timing:

- t+0:  container start
- t+~6s: `Initializing rtloader` (triggered by the discoverer's first call)
- t+~10s: krakend check `[OK]`, scraping metrics.

The smoke test just needs to wait until `agent status` shows the krakend
section. ~30 s is a comfortable upper bound.

### 7. Verify the krakend check is scheduled with the discovered config

```bash
docker exec dd-agent-foo agent configcheck 2>&1 | grep -B1 -A12 "krakend "
```

Expected:

```
=== krakend check ===
Configuration provider: file
Configuration source: file:/etc/datadog-agent/conf.d/krakend.d/auto_conf_discovery.yaml
Config for instance ID: krakend:<digest>
openmetrics_endpoint: http://<krakend-container-ip>:9090/metrics
~
Auto-discovery IDs:
* krakend
```

The `openmetrics_endpoint` must reference the krakend container's IP on
`docker_default` (172.20.0.x by default) and port 9090. That URL was
synthesized inside Python `discover()` — it is the proof that the bridge
round-trip worked.

```bash
docker exec dd-agent-foo agent status 2>&1 | sed -n '/krakend (/,/^[A-Z][a-z]*$/p'
```

Expected:

```
krakend (1.4.1)
---------------
  Instance ID: krakend:<digest> [OK]
  Configuration Source: file:/etc/datadog-agent/conf.d/krakend.d/auto_conf_discovery.yaml[0]
  Total Runs: <N>
  Metric Samples: Last Run: 44, Total: <N*44>
  ...
```

The `[OK]` and the non-zero metric-sample count are the two assertions a
smoke test should make.

For positive-evidence cross-checking, the Python check log line is:

```
docker logs dd-agent-foo | grep "Scraping OpenMetrics endpoint"
# krakend:<digest> | (base.py:69) | Scraping OpenMetrics endpoint: http://172.20.0.X:9090/metrics
```

### 8. Cleanup

```bash
docker rm -f dd-agent-foo
cd /home/vagrant/go/src/github.com/DataDog/integrations-core/krakend/tests/docker
KRAKEND_VERSION=2.10 docker compose down
```

## Negative scenarios worth automating later

The original krakend experiment plan (now superseded) defined three
scenarios. Of these, only the first was run end-to-end here. The others
are still valuable smoke targets and an automated harness should cover
them:

1. **Default port (9090) — covered above.** Hint matches first; check
   scheduled with the hint port.

2. **Non-default port.** Edit `krakend.json` and the compose file to
   listen on a non-9090 port (e.g. 9000). On AD reconcile, the krakend
   `discover()` should fall back to scanning the rest of the container's
   exposed ports and find 9000. Resulting `openmetrics_endpoint` should
   reference :9000.

3. **Negative case.** Start a non-krakend container labelled with
   `com.datadoghq.ad.check_names='["krakend"]'` (e.g. `nginx:alpine`).
   `discover()` probes /metrics, gets a non-Prometheus response, returns
   `None`. No krakend check should be scheduled. Logs show the
   "discover did not match" debug line, no `[ERROR]` for krakend.

## Pitfalls encountered (each is a real failure mode the harness must avoid)

### "Configuration file contains no valid instances"

If `auto_conf_discovery.yaml` has `instances: []` (or no `instances:`)
and no `discovery: {}` block, `comp/core/autodiscovery/providers/config_reader.go`
rejects the file. Plan B Task 11 added `cf.Discovery == nil` to the empty-
instances guard. The current krakend file has `discovery: {}` and
`instances: []` — both must be present.

### "agent: error while loading shared libraries: libdatadog-agent-rtloader.so.0.1.0"

The agent binary's RUNPATH is the absolute host path of `dev/lib`. The
bind mount must use that exact path on the container side too:

```
-v "<repo>/dev/lib:<repo>/dev/lib:ro"
```

Not `:/opt/datadog-agent/embedded/lib:ro`. Different RUNPATH, different
binary.

### "Could not initialize Python: ... libpython3.12.so.1.0: cannot open shared object file"

The host-built rtloader was linked against Python 3.12 (system headers).
The container ships Python 3.13. Use `dda inv rtloader.install-with-bazel`
to get a 3.13-linked rtloader, then copy the .so files from
`dev/embedded/lib/` into `dev/lib/` so the agent finds them via its
RUNPATH.

### "Could not initialize Python: ... libdatadog-agent-three.so: cannot open shared object file"

The bazel-built `libdatadog-agent-rtloader.so` has RPATH
`<repo>/dev/embedded/lib`. That's the path Bazel installs `three.so` to.
The agent container can't find it unless `dev/embedded` is also bind-
mounted at the same absolute path:

```
-v "<repo>/dev/embedded:<repo>/dev/embedded:ro"
```

### Krakend not appearing in `agent configcheck`

If everything else is right but `agent configcheck` does not list krakend,
the bind mount of `auto_conf_discovery.yaml` into `/etc/datadog-agent/conf.d/krakend.d/`
is probably missing. The agent does NOT pick autoconfig files up from the
Python package's `data/` directory at runtime (only at install time, via
omnibus packaging).

### Python init timing

The discoverer triggers `InitPython` itself via the shared `pythonOnce`
when `python_lazy_loading` is true (default). The same idempotent
sync.Once is also held by the python check loader, so multiple
consumers can race safely. `Initializing rtloader` should appear
exactly once per agent process.

### "endpoint with name dd-agent-foo already exists in network docker_default"

Benign. The agent stays connected to the network across `compose down`/`up`
because the network itself isn't removed if other containers (the agent)
are still attached. The new krakend container joins the same network, so
the agent reaches it on the existing connection.

## Build/test commit IDs at the time of the successful run

For reproducibility:

- `datadog-agent` head: f714c5e5cc2 (Plan B + final review fixups +
  rebase onto main + lazy-init Python from the discoverer bridge).
- `integrations-core` head: de98ae4025 (Plan A + Plan B Task 4 + krakend
  migration + changelog).

Both branches are local-only (not pushed to `origin`). To reproduce, push
or check out at those SHAs.

## Note: `dev/lib/` rtloader needs to be restored after every agent rebuild

`dda inv agent.build` rebuilds rtloader via cmake, which links against
the host system's Python (3.12 in the dev container). The agent
container ships Python 3.13 and won't load that .so. After every
rebuild, restore the bazel-built (Python 3.13-linked) rtloader:

```bash
cp -P dev/embedded/lib/libdatadog-agent-rtloader* dev/lib/
cp -P dev/embedded/lib/libdatadog-agent-three.so dev/lib/
```

`dev/embedded/` is populated once by `dda inv rtloader.install-with-bazel`;
the build chain doesn't touch it.
