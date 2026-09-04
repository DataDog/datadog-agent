# Agent Check Runner

Agent Check Runner (ACR) is an optional out-of-process Python check executor. It runs the
Python integration checks the Core Agent runs today, but in a separate process so the Core
Agent's own scheduling, aggregation and forwarding loops are isolated from check work.

This document covers the **Agent-side integration surface** in this repository: configuration,
supervision, ports and remote-agent registration. It is the companion to
[`agent-data-plane.md`](agent-data-plane.md), which covers ADP. ACR follows the same pattern
with two structural differences:

- ADP ships a single static binary; ACR ships a **directory** (`embedded/bin/agent-check-runner/`)
  that must move as a unit because it resolves its Rust stdlib through an `$ORIGIN` rpath and its
  adapter plugins relative to `current_exe()`. The packaging details live in the ACR
  repository's packaging design doc.
- ACR has no preflight mode. There is no `comp/checkrunner/preflightmode` equivalent of
  `comp/dataplane/preflightmode`; ACR is either bundled and enabled, absent, or disabled.

## Configuration

ACR's settings live under the `check_runner` section of `datadog.yaml`, declared in
`pkg/config/schema/yaml/core_schema.yaml` and generated into `pkg/config/setup` like every other
setting:

| Setting | Type | Default | Notes |
|---|---|---|---|
| `check_runner.enabled` | boolean | `false` | When `false` and ACR is not in standalone mode, ACR registers, receives its initial configuration, then exits cleanly. The s6 service stays up but the process is gone. |
| `check_runner.standalone_mode` | boolean | `false` | When `true`, ACR does not register with the Core Agent and reads the config file directly, skipping the config stream. Used by ACR's own test harness and standalone deployments, not the container image. |
| `check_runner.api_listen_address` | string | `tcp://0.0.0.0:5200` | Unprivileged API. Must include a URL scheme. |
| `check_runner.secure_api_listen_address` | string | `tcp://0.0.0.0:5201` | Privileged API, served with the Core Agent IPC TLS config. Must include a URL scheme. |

These settings are **undocumented** (internal), mirroring `data_plane.*`: they are not rendered
into `datadog.yaml.example` or the public docs while ACR is not GA.

### How ACR receives configuration

When not in standalone mode, ACR connects to the Core Agent's secure IPC API, registers as a
remote agent, and subscribes to the **config stream**. The Core Agent streams its full resolved
configuration (`AllFlattenedSettingsWithSequenceID` in `comp/core/configstream/impl/configstream.go`)
to every registered remote agent; ACR deserializes only the keys it knows about. This is the same
mechanism ADP uses.

Two existing top-level settings flow to ACR over the stream without any new declaration:

- `confd_path` (default `${conf_path}/conf.d`) — the autodiscovery `conf.d` directories ACR
  loads check configurations from. ACR reads this as its `checks_config_dir`.
- The Core Agent's IPC auth settings (`ipc_*`) — ACR uses these to authenticate the secure API
  and to connect back to the Core Agent.

No new `checks_config_dir` setting is needed; ACR reads `confd_path`.

## Supervision

In the container image, ACR is an s6 service, not a process the Core Agent launches directly
(unlike ADP, which the Core Agent starts itself for preflight mode). The service files live in
`Dockerfiles/agent/s6-services/check-runner/`:

- **`run`** — guards on `s6-test -x` against the nested binary path
  `embedded/bin/agent-check-runner/agent-check-runner`. If the binary is absent the service logs
  a disablement message and calls `s6-svc -d` to bring itself down. This is the same self-disable
  idiom the `data-plane` service uses.
- **`finish`** — turns a clean exit into permanent disablement, so an ACR that exits because
  `check_runner.enabled` is `false` does not get restarted by s6's one-to-one strategy. This
  is the same trap the `data-plane` `finish` script handles.

`Dockerfiles/agent/entrypoint.d/agent-check-runner` mirrors `entrypoint.d/agent-data-plane`: an
`[[ -x ]]` guard then `exec`. `Dockerfiles/agent/entrypoint.d/simple-all-in-one` appends
`agent-check-runner` to the all-in-one service list, and `Dockerfiles/agent/Dockerfile` removes
the s6 service directory when the binary is not bundled, before the world-writable chmod sweep.
See [`SUPERVISION.md`](../../Dockerfiles/agent/SUPERVISION.md) for the service catalogue.

`/probe.sh` runs `agent health`, which only sees in-process registrants of `pkg/status/health`.
A crash-looping ACR does **not** make the container unhealthy — do not treat a green pod as
evidence that ACR is running.

## Ports

ACR listens on two ports, both derived from the `check_runner.*_listen_address` settings:

| Port | Setting | Surface |
|---|---|---|
| 5200 | `check_runner.api_listen_address` | Unprivileged API |
| 5201 | `check_runner.secure_api_listen_address` | Privileged API (TLS) |

These are address strings (`tcp://host:port`), not integer `*_port` keys, so they are not
scanned by the `agent diagnose` port suite directly. `pkg/diagnose/ports/ports.go` lists
`agent-check-runner` in `agentNames` so that a port bind owned by the `agent-check-runner`
process is recognized as agent-owned rather than flagged as a foreign conflict.

## Remote Agent Registry

ACR registers with the Core Agent's Remote Agent Registry (RAR) before it does anything else.
Registration is implemented on the ACR side in `bin/agent-check-runner/src/internal/remote_agent.rs`
(`RemoteAgentBootstrap::from_configuration`); it is gated by `check_runner.standalone_mode`
(standalone skips it entirely).

ACR advertises two gRPC services during registration, both of which the RAR already recognizes
without any Agent-side allowlist:

- `datadog.remoteagent.status.v1.StatusProvider` — fans ACR's status into `agent status` and
  flares.
- `datadog.remoteagent.telemetry.v1.TelemetryProvider` — fans ACR's internal telemetry into the
  Agent's telemetry pipeline.

ACR does **not** advertise a `FlareProvider` service; the RAR treats flare as optional. The RAR
is generic: any remote agent that registers with the recognized service names is accepted, so no
Agent-side code change is required for ACR's registration. This is the same precedent ADP set.

The registration is maintained for the process lifetime by a background worker
(`acr-remote-agent-registration`), which refreshes on a 30s interval (5s on failure).

## Binary path

There is no Go constant for ACR's binary path today. ACR is s6-supervised in the container and
the service scripts reference the path directly. ADP has `defaultpaths.GetDefaultDataPlaneBin`
because the Core Agent launches ADP itself for preflight mode (`comp/dataplane/preflightmode`);
ACR has no equivalent Go launcher yet. Add a `GetDefaultCheckRunnerBin` accessor to
`pkg/util/defaultpaths` when a Go caller appears (for example, a procmgr migration entry in
`pkg/procmgr/coat/services.go`).

## Open items

- **Duplicate execution** (running the same checks in both the Core Agent and ACR) is a GA
  concern, not a first-deploy concern. The nearest lever is `integration.excluded` →
  `IsCheckAllowed` in `pkg/collector/infra_mode.go`.
- **Enablement** lives outside this repository: the Helm chart and datadog-operator must set the
  container spec and env, as they do for `DD_DATA_PLANE_ENABLED`.
