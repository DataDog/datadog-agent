# Local health validation

Retained configurations for `TestLinuxHealthSuiteOnLocal/TestDefaultInstallHealthy`.
The test body is unchanged. Do not run the entire local health suite: its unhealthy
subtest still requests a Pulumi/EC2 environment update.

## Result matrix

The exact unchanged health subtest passed for all six source/receiver combinations:

| Agent source | Fakeintake | Blackhole | Datadog / dddev |
|---|---|---|---|
| Locally built core binary | PASS | PASS | PASS (fresh environment) |
| Pipeline `138372337` DEB installed in Docker | PASS | PASS | PASS (fresh environment) |

Both sources also passed an in-place **fakeintake → blackhole → fakeintake** loop.
Native RC transitions are not supported in place; the expected rejection and fresh
native environment are recorded rather than bypassed. These are core-only health
checks. Package native HTTP acceptance was observed separately, not inferred from health.

## Completed: locally built core Agent

| Step | Result |
|---|---|
| Build/install using `local-binary-fakeintake.yml` | PASS |
| Health test against fakeintake | PASS |
| Receiver-only switch to `local-binary-blackhole.yml` | PASS |
| Same health test against blackhole | PASS |
| Receiver-only switch back to fakeintake | PASS |
| Same health test against fakeintake again | PASS |
| Fresh environment using `local-binary-datadog.yml`, same health test | PASS |
| Normal cleanup | PASS |

Commands were run from the repository root, using a Bazel-built `e2ectl` and a
private `E2ECTL_HOME`. These configs use the simplified agent surface
(`source: true` builds from the invocation root; `pipeline: 138372337` downloads
the exact DEB through the pipeline provider, whose receipt carries the digest
that these pre-simplification runs recorded as a hand-typed `sha256`). No
credentials appear in these configs.

```sh
e2ectl start --config test/new-e2e/tests/agent-subcommands/validation/local-binary-fakeintake.yml --name health-loop-zuyfog
e2ectl install --env health-loop-zuyfog

E2ECTL_ENV=health-loop-zuyfog dda inv -- new-e2e-tests.run \
  --targets=./tests/agent-subcommands/ \
  --run='^TestLinuxHealthSuiteOnLocal$/^TestDefaultInstallHealthy$' \
  --timeout=3m

# No manual sink: the local base manages it.
e2ectl receiver apply --env health-loop-zuyfog \
  --config test/new-e2e/tests/agent-subcommands/validation/local-binary-blackhole.yml
# Repeat the same test command, then apply local-binary-fakeintake.yml and test again.
```

The original recorded loop ran the sink by hand: `e2ectl receiver serve
--type blackhole --listen 0.0.0.0:8080` in a read-only, unprivileged Docker
container on `health-loop-zuyfog-net`, named `health-loop-zuyfog-blackhole`, and
pasted its DNS name into the config's `blackhole.url`. That operator-owned
`url` escape hatch still works; the retained blackhole configs now use the
**managed selection** (`receiver: type: blackhole`, no url), which was
re-validated on 2026-09-21 with a Bazel-built `e2ectl` in `/tmp/e2ectl-managed-bh`:
`start` + `install` started the sink container automatically
(`managed-bh-blackhole`, read-only, unprivileged, no host port), the Agent's
effective `dd_url` was `http://managed-bh-blackhole:8080`, the forwarder showed
successful `series_v2`/`check_run_v1`/`intake` transactions against it, the same
health test passed, `receiver apply` back to fakeintake removed the sink, and
`stop` left zero containers, networks, volumes or store entries. The managed
sink is local-base only; remote bases keep the `url` escape hatch.
API/application keys used by the test harness were dummy values for these runs.
Health PASS is process-health evidence, not a claim of real-backend ingestion.

`local-binary-results.json` retains the commands, per-step durations and four
health-PASS observations, including the native run authorized for dddev. Private
logs are under `/tmp/e2ectl-health-loop.zUYfog`; no secrets or raw status/config
output are committed. Both temporary binary environments have been stopped.

## Pipeline package: exact existing DEB

`pipeline-138372337.json` records the exact arm64 DEB version, local cache path and
SHA256 obtained with:

```sh
dda inv -- package.download --pipeline=138372337 --type=deb --arch=arm64 \
  --no-extract --path="$HOME/.cache/e2ectl-validation/pipeline-138372337/arm64"
```

The local container target of the merged `package` installer consumes this exact
existing package without building Agent source. It performs a real dpkg
installation inside an Ubuntu Docker filesystem, but runs only core foreground.
The SSH/systemd host target of the same installer and its receiver capability gate
are unchanged. No package `ProducerProfile` is created for the container target:
this is explicitly a **core-health/configuration** target scope, not full
package/service validation or all-signal routing attestation.

> The recorded runs below used the pre-merge `package-core` installer section,
> then the merged installer section (`install: package` / `package:` with a
> hand-downloaded `existing-package`), before the simplified surface landed
> (`pipeline: 138372337` with provider-pinned digest). `local-package-results.json`
> retains the original section name as executed. The container-target behavior is
> unchanged across all three shapes.

## Completed: pipeline package matrix

Actual inputs are `local-package-fakeintake.yml`, `local-package-blackhole.yml`, and
`local-package-datadog.yml`. The rejected same-environment RC transition input is
also retained as `local-package-native-transition-rejected.yml`.
`local-package-blackhole.yml` was updated to the managed selection (no `url`)
after these recorded runs; the recorded package loop itself ran the sink
operator-owned via `url`. The managed sink's lifecycle was re-validated end to
end on the binary target (same local base and sync path); the package
container target consumes the identical sync before its install.
Structured commands, durations, package/runtime/core
checksums, effective endpoint/disabled-feature observations, Docker Host output,
AgentBinPath, volume identities, prior failures and cleanup evidence are retained
in `local-package-results.json`.

| Step | Result | Duration |
|---|---|---|
| Install DEB into Docker, isolated dummy-key health/CPU/config preflight | PASS | 120.06 s |
| Unchanged health test against fakeintake | PASS | 31.50 s |
| In-place receiver apply to blackhole | PASS | 36.79 s |
| Same health test against blackhole | PASS | 31.35 s |
| In-place receiver apply back to fakeintake | PASS | 36.85 s |
| Same health test against fakeintake again | PASS | 33.16 s |
| Disabled→native RC transition in the same environment | Rejected as expected, state preserved | 0.12 s |
| Fresh native environment, same DEB installation | PASS | 40.50 s |
| Same health test against authorized native backend | PASS | 30.99 s |
| Native forwarder observation | 9 successful HTTP transactions, 0 TLS errors | bounded sample |
| Normal cleanup and independent resource absence checks | PASS | see JSON |

Fakeintake returned two `system.cpu.user` payloads from
`health-package-138372337-agent`. This proves delivery of that metric, not all-signal
coverage. Native forwarder counters showed three accepted `series_v3`, three
`check_run_v1` and three `intake` requests. **HTTP acceptance is not an independent
query for indexed metrics.** Receiver status continues to say delivery `unverified`.

The real-backend case was authorized for the **dddev organization**: site
`datadoghq.com`, organization selected by the configured `runner/api_key`. No key
is retained here. CLI install/apply must use that runner key, not a dummy
`E2E_API_KEY`; only the test harness used dummy API/application keys to avoid test
telemetry credentials. The parent also recorded native source-binary health PASS
in `/tmp/e2ectl-health-loop.zUYfog/native-binary-result.json`; the earlier binary
configs/results are preserved.

### Reproduce the bounded package loop

Use a Bazel-built `e2ectl`, a private `E2ECTL_HOME`, the retained DEB, and native
Linux Docker with `docker.io/library/ubuntu:24.04` available. Standard Ubuntu
`ca-certificates` are installed in a credential-free prerequisite layer. The exact
DEB/postinst step and the initial dummy probe have networking disabled;
`policy-rc.d` blocks automatic service starts. No TLS bypass, custom CA, privileged
container, host `/opt` mount, Docker socket mount, or host package install is used.
The container-target config uses the standard receiver-validated overlay;
re-enabling core-only pins, integrations and subagents is rejected.

```sh
export E2ECTL_HOME=/path/to/private/package-store
# For native CLI operations, do not override the configured runner key with a dummy.
e2ectl start --config test/new-e2e/tests/agent-subcommands/validation/local-package-fakeintake.yml \
  --name health-package-138372337
e2ectl install --env health-package-138372337

E2ECTL_ENV=health-package-138372337 \
  E2E_API_KEY=00000000000000000000000000000000 \
  E2E_APP_KEY=0000000000000000000000000000000000000000 \
  dda inv -- new-e2e-tests.run --targets=./tests/agent-subcommands/ \
  --run='^TestLinuxHealthSuiteOnLocal$/^TestDefaultInstallHealthy$' --timeout=3m

# The managed blackhole sink starts itself on install/apply (no manual step).
e2ectl receiver apply --env health-package-138372337 \
  --config test/new-e2e/tests/agent-subcommands/validation/local-package-blackhole.yml
# Repeat the same health command, apply local-package-fakeintake.yml, test again.
# stop removes the managed sink with the Agent, fixture, volume and network.
e2ectl stop --env health-package-138372337

# Fresh environment for native RC; real runner key is resolved only after preflight.
e2ectl start --config test/new-e2e/tests/agent-subcommands/validation/local-package-datadog.yml \
  --name health-package-native-138372337
e2ectl install --env health-package-native-138372337
# Repeat the exact health command above with E2ECTL_ENV=health-package-native-138372337.
# Separately observe non-secret Agent forwarder counters; never log raw config/status.
e2ectl stop --env health-package-native-138372337
```

Final private logs and the runnable loop script are under
`/tmp/e2ectl-package-health.LV4HNI`. The package and all configs are retained.
All owned environments, containers, networks and runtime volumes were removed;
five credential-free installation image identities from the attempts remain in
Docker's cache for inspection (listed in the JSON).

### Prior failure retained: health is not forwarding

`/tmp/e2ectl-package-health.R6zbjn` initially had all health assertions PASS, but
an extra forwarder observation found **10 TLS errors and zero successes** because
bare Ubuntu had no system CA bundle. This was not accepted as a successful native
case. Its counters, original image identity and correction are retained separately
in the JSON and private `native-acceptance-correction.json`. The implementation now
installs standard CA certificates, requires the CA bundle on runtime verification,
rejects TLS-bypass/custom-CA input, and the complete matrix above was rerun with new
image identities. Earlier BuildKit base-ID and blackhole-YAML setup errors are
also recorded rather than hidden.

