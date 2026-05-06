# discovery-dev image

Local-only image used by the krakend discovery e2e test in
integrations-core, until the agent-side discovery code is merged to
nightly. Bakes the locally-built agent binary, rtloader, and
bazel-built embedded Python 3.13 onto `nightly-main-py3-jmx`,
mirroring the host path of `dev/lib` and `dev/embedded` inside the
image so RUNPATH/RPATH still resolve.

## Build

Prerequisites (see `docs/superpowers/2026-05-06-discover-e2e-smoke.md`):
- `dda inv agent.build --build-exclude=systemd`
- `dda inv rtloader.install-with-bazel`
- copy bazel artifacts from `dev/embedded/lib/` into `dev/lib/`

Then:

```
dda inv discovery-dev.build-image
```

## Use

```
DDEV_E2E_AGENT=datadog/agent-dev:discovery-local DDEV_E2E_DOCKER_NO_PULL=1 \
  ddev env start --dev krakend discovery
```

`DDEV_E2E_DOCKER_NO_PULL=1` is required so ddev does not try to pull
the local-only tag.
