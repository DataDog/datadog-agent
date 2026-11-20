# Embedded supervisor for the Agent image

The Agent image runs several binaries side by side: the core `agent` plus auxiliary
processes such as the trace agent, the process agent and system-probe. To ship them in one
container, the image includes `dd-procmgrd`, the Datadog process manager.

When the `ENTRYPOINT` environment variable is not set, `/opt/entrypoints/_default` runs
`/opt/entrypoints/procmgr-all-in-one` when `dd-procmgrd` is executable and
`/opt/datadog-agent/processes.d/aio` exists. Otherwise it runs `s6-all-in-one`. That is how
`agent-ddot` stays on s6 when it copies these entrypoints onto an older base that has no aio
catalog. `procmgr-all-in-one` runs the cont-init scripts, then execs `dd-procmgrd`.
`dd-procmgrd` is PID 1, or the container's main process when the container shares the host
PID namespace (`pid: host`). It reads its process catalog from
`/opt/datadog-agent/processes.d/aio/`. When a custom service is added, the entrypoint copies
that catalog and the custom service into a staging directory and reads that instead.

## Processes

Each process starts only when its binary exists in the image and its configuration gate is
open. A process whose gate is closed is listed as `Skipped`, with the reason.

| Process | Binary | Starts when |
|---------|--------|-------------|
| `datadog-agent` | `agent` | Always. It is critical: when it exits, the container exits |
| `datadog-agent-trace` | `trace-loader`, then `trace-agent` | `apm_config.enabled` or `apm_config.error_tracking_standalone.enabled` |
| `datadog-agent-process` | `process-agent` | Any `process_config` collection setting, or `network_config.enabled` or `system_probe_config.enabled` in `system-probe.yaml` |
| `datadog-agent-sysprobe` | `system-probe` | `network_config.enabled`, `system_probe_config.enabled` or `runtime_security_config.enabled` in `system-probe.yaml`, or `software_inventory.enabled`, unless `system_probe_config.external` is set |
| `datadog-agent-security` | `security-agent` | `compliance_config.enabled`, or `runtime_security_config.enabled` in `system-probe.yaml` |
| `datadog-agent-otel` | `otel-agent` | Always. `otel-agent` exits 0 on its own when `otelcollector.enabled` is false |
| `datadog-agent-data-plane` | `agent-data-plane` | `data_plane.enabled` |
| `datadog-agent-par` | `privateactionrunner` | `private_action_runner.enabled` |
| `datadog-agent-par-control` | `par-control` | `private_action_runner.split_enabled`, unless `fips.enabled` is set |
| `datadog-agent-action-executor` | `privateactionrunner` | Never at boot. `par-control` starts it on demand |

Auxiliary processes restart after a non-zero exit or a signal. The first wait is
`restart_sec` (2 seconds). The delay doubles up to 60 seconds while the process exits within
1 second of starting, and it returns to 2 seconds after a run of at least 1 second. The
shipped `start_limit_burst: 5` and `start_limit_interval_sec: 10` do not stop that loop. A
process that exits 0 stays stopped. `datadog-agent-otel` restarts 2 seconds after a failure,
with no limit. `datadog-agent` and `datadog-agent-action-executor` never restart. Every
process inherits the container `PATH`. `DD_PM_*` variables are not passed to the processes,
except `DD_PM_SOCKET_PATH`.

## Cont-init scripts

Before `dd-procmgrd` starts, the entrypoint runs every regular file in `/etc/cont-init.d/`,
in C-locale alphabetical order. It first copies them to a staging directory, makes the copies
executable, and runs the copies. The staging directory is the first of these where a probe
script can execute:

1. `/var/run/s6/procmgr-cont-init.d`
2. a temporary directory under `${TMPDIR:-/tmp}`
3. a temporary directory under `/opt/datadog-agent/run`

A temporary directory is removed after cont-init. The log shows the chosen directory in a
`===== STAGING cont-init in <dir> =====` line, and each script in a
`===== RUNNING /etc/cont-init.d/<name> =====` line.

Rules for custom scripts:

- The `90-` to `99-` prefixes are reserved for custom scripts, which run after the provided ones.
- A script must start with a shebang (for example `#!/bin/bash`), because it is executed directly.
- A failing script stops the container before `dd-procmgrd` starts. The container exits with
  the script's exit code.
- Environment variables exported in a script do not reach the Agent. Set container
  environment variables instead.

A script runs as a background child of the entrypoint, in its own session, so it starts
with SIGINT and SIGQUIT ignored. Stopping the container during cont-init sends SIGTERM to
that session, including a child the script forked. The entrypoint waits for the script to exit
before it exits. A script that ignores SIGTERM therefore delays the stop until Docker's stop
timeout, and the container then exits with code 137.

You can use custom scripts to:

- `pip install` Python dependencies for your custom checks
- append content at the end of `/etc/datadog-agent/datadog.yaml`
- disable standard checks by editing the files in `/etc/datadog-agent/conf.d/`

## Custom services

A custom service is a directory under `/etc/services.d/` other than the ones the image ships
(`agent`, `data-plane`, `otel`, `privateactionrunner`, `process`, `security`, `sysprobe`,
`trace`). Each custom directory that contains a `run` file is started, except when its name
is not a process name or it matches a catalog process such as `datadog-agent`. The entrypoint
copies the whole service directory and runs that copy. The working directory is the staged
copy, not `/etc/services.d/<name>`. The service uses `restart: always`, so it restarts 1
second after every exit, including exit 0, with no limit. A `down` file leaves it stopped
until `dd-procmgr start <name>`. The shipped directories are already in the catalog, so their
`run` files are not started again.

A `finish` script is not run. `notification-fd` and a nested `log/` directory are not used.
Editing the `run` file of a shipped service has no effect. `ENTRYPOINT=s6-all-in-one` keeps
the s6 behavior.

The copy is staged under `/var/run/s6` when that directory is writable and executable.
Otherwise it is staged under `${TMPDIR:-/tmp}`, then under `/opt/datadog-agent/run`.
`/opt/datadog-agent/run` is a VOLUME. A graceful stop removes the fallback directory this
start created. If `dd-procmgrd` is killed before that, the directory remains until the
volume is removed.

## Useful commands

`dd-procmgr` talks to the supervisor. It finds the socket on its own, so plain `docker exec`
works:

```
docker exec <container> dd-procmgr list
docker exec <container> dd-procmgr describe datadog-agent-process
docker exec <container> dd-procmgr stop datadog-agent-process
docker exec <container> dd-procmgr start datadog-agent-process
```

To restart a process, run `stop` then `start`. Stopping the core `datadog-agent` stops the
container with exit code 0, so restart the container instead.

## Exit codes and stopping

- When the core Agent exits, `dd-procmgrd` stops the other processes and the container exits
  with the Agent's exit code, or 128 plus the signal number when a signal killed it (137 after
  `SIGKILL`).
- On `docker stop`, `dd-procmgrd` asks each process to stop and gives it up to its
  `stop_timeout` (90 seconds by default, 180 seconds for `datadog-agent-par-control`) before
  it kills it. A clean shutdown exits 0. The container runtime still kills everything when its
  own grace period ends (Docker's default is 10 seconds), and the container then exits 137.
- `docker stop` during a cont-init script exits 143, because the TERM handler runs. An
  interrupt (INT) before `dd-procmgrd` starts also exits 143.
- A recommended grace period has not been measured yet.

## Differences from s6

These differences from the s6 image are accepted:

- A custom `/etc/services.d/<name>/finish` script is not run, and a replacement `run` file in a
  shipped service directory is ignored. `ENTRYPOINT=s6-all-in-one` keeps both.
- Scripts in `/etc/cont-finish.d/` and `/etc/fix-attrs.d/` do not run.
- `S6_*` variables do not control supervision.
- `s6-svc` and `s6-svstat` do not control `dd-procmgrd`. `dd-procmgr` replaces them.
- `DD_PM_*` variables are not passed to the processes, except `DD_PM_SOCKET_PATH`.
- Start, exit, and restart messages stay in the container output and are no longer appended to
  `/var/log/datadog/init.log`. They are worded differently.
- An auxiliary process that crashes restarts after 2 seconds, and the delay doubles up to 60
  seconds while it keeps exiting within 1 second of starting. `datadog-agent-otel` still
  restarts every 2 seconds. Under s6, every crash restart waited 2 seconds.

## Rolling back to s6

For this release, the s6 supervisor is still in the image. To select it, set the environment
variable `ENTRYPOINT=s6-all-in-one`. `ENTRYPOINT` is an environment variable read by the
image, not Docker's `--entrypoint`. Do not override the container entrypoint or command.

- Docker: `docker run -e ENTRYPOINT=s6-all-in-one ...`
- Compose: `environment: [ENTRYPOINT=s6-all-in-one]`
- ECS: an `environment` entry `{"name": "ENTRYPOINT", "value": "s6-all-in-one"}` in the
  Agent container definition
- Kubernetes: `env: [{name: ENTRYPOINT, value: s6-all-in-one}]`. For the Agent sidecar, use
  the profile `env` override.

Images older than the release that added `s6-all-in-one` exit 1 with
`"s6-all-in-one" is not a valid ENTRYPOINT.` when the variable is set. Remove it before downgrading. s6 will be removed in a future release.
