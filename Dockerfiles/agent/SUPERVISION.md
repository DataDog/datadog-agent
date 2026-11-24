# Embedded supervisor for the Agent image

The Linux Agent image runs its processes under `dd-procmgrd`, the Datadog process manager,
instead of s6.

## Entrypoint selection

When `ENTRYPOINT` is not set, `/opt/entrypoints/_default` runs
`/opt/entrypoints/procmgr-all-in-one` if `dd-procmgrd` is executable and the marker
`/opt/datadog-agent/processes.d/aio/.procmgr-default` exists. Otherwise it runs
`s6-all-in-one`. `agent-ddot` copies these entrypoints onto an older base that has the
catalog but not the marker, so that base stays on s6.

`procmgr-all-in-one` runs the cont-init scripts, then execs `dd-procmgrd`. When there are
custom services or shipped `down` files, it first stages a copy of the catalog and the
custom services. Otherwise the daemon reads the catalog in the image. The daemon is PID 1,
or the main process when the container uses `pid: host`.

## Processes

Each process starts only when its binary is in the image and its configuration gate is open.
A closed gate shows as `Skipped` with the reason.

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

Auxiliary processes restart after a non-zero exit or a signal. The first wait is 2 seconds.
The delay doubles up to 60 seconds while the process exits within 1 second of starting, and
returns to 2 seconds after a run of at least 1 second. The catalog start limit does not stop
that loop. A process that exits 0 stays stopped. `datadog-agent-otel` restarts 2 seconds after
a failure, with no limit. `datadog-agent` and `datadog-agent-action-executor` never restart.

Every process inherits the container `PATH`. `DD_PM_*` variables are not passed to the
processes, except `DD_PM_SOCKET_PATH`.

## Cont-init scripts

Before `dd-procmgrd` starts, the entrypoint copies each regular file in `/etc/cont-init.d/`
(C-locale order) to a staging directory, makes the copies executable, and runs them. The
staging directory is the first of `/var/run/s6`, `${TMPDIR:-/tmp}`, or `/opt/datadog-agent/run`
where a probe script can execute. A temporary staging directory is removed after cont-init.

- Use the `90-` to `99-` prefixes for custom scripts. They run after the provided ones.
- Start each script with a shebang, because it runs directly. A failing script stops the
  container before `dd-procmgrd` starts, with the script's exit code.
- Variables exported in a script do not reach the Agent. Set container environment variables
  instead.
- Scripts run with `S6_KEEP_ENV=1`, so a `#!/usr/bin/with-contenv` shebang sees the container
  environment even with `S6_KEEP_ENV=0`.
- Scripts run in their own session and process group with SIGINT and SIGQUIT ignored.
  Stopping the container during cont-init sends SIGTERM to that process group, including
  children it forked, and the entrypoint waits for them to exit. A child that moves to its own
  process group is not signaled. A script that ignores SIGTERM gets SIGKILL 5 seconds
  later, so the container exits with code 143 before Docker's default 10-second timeout.
  A `docker stop -t` under 5 seconds ends the container first, with code 137.

## Custom services

A directory under `/etc/services.d/` that is not a shipped name and contains a `run` file
starts as a custom service. Its name must be a valid process name and must not match a catalog
process. The entrypoint copies the whole directory and runs the copy, so the working directory
is the staged copy. Links inside it are copied as links, so a relative link must point inside
the service directory or be replaced by an absolute one. `run` itself is copied as a file.
Custom services use `restart: always` (1 second after every exit, no limit) and run with
`S6_KEEP_ENV=1`. A `down` file leaves one stopped until `dd-procmgr start <name>`.

The shipped directories (`agent`, `data-plane`, `otel`, `privateactionrunner`, `process`,
`security`, `sysprobe`, `trace`) are already in the catalog, so their `run` files are not
started again. A `down` file in one keeps its catalog process stopped at start until
`dd-procmgr start <process>`, for example `datadog-agent-trace`. `privateactionrunner` covers
`datadog-agent-par` and `datadog-agent-par-control`. The `agent` down file is ignored with a
warning, because the core Agent always starts.

Not supported: `finish` scripts (not run), `notification-fd`, and nested `log/` directories.
Edits to a shipped service's `run` file have no effect. `ENTRYPOINT=s6-all-in-one` keeps the
s6 behavior.

A staged copy under `/opt/datadog-agent/run` (a VOLUME) is removed on a graceful stop. If
`dd-procmgrd` is killed first, the directory stays until the volume is removed.

## Useful commands

`dd-procmgr` finds the supervisor socket on its own, so plain `docker exec` works:

```
docker exec <container> dd-procmgr list
docker exec <container> dd-procmgr stop datadog-agent-process
docker exec <container> dd-procmgr start datadog-agent-process
```

To restart a process, run `stop` then `start`. Stopping `datadog-agent` exits the container
with code 0, so restart the container instead.

## Exit codes and stopping

- When the core Agent exits, the other processes stop and the container exits with the Agent's
  exit code, or 128 plus the signal number when a signal killed it (137 after `SIGKILL`).
- On `docker stop`, the processes stop concurrently: each gets its `stop_timeout` (90 seconds
  by default, 180 seconds for `datadog-agent-par-control`) from the same moment, so the
  shutdown takes about the longest timeout, not the sum. A process still alive after its
  timeout is force-killed, and a process that survives the force-kill can add up to 10 seconds
  before the container gives up on it. A clean shutdown exits 0. If processes are still running
  when Docker's default 10-second grace period ends, the container exits 137.
- `docker stop` during cont-init exits 143. An interrupt (INT) before `dd-procmgrd` starts
  also exits 143.
- A recommended grace period has not been measured yet.

## Differences from s6

- `finish` scripts, and scripts in `/etc/cont-finish.d/` and `/etc/fix-attrs.d/`, do not run.
- `S6_*` variables do not control supervision. `s6-svc` and `s6-svstat` do not control
  `dd-procmgrd`. `dd-procmgr` replaces them.
- `DD_PM_*` variables are not passed to the processes, except `DD_PM_SOCKET_PATH`.
- Start, exit, and restart messages stay in the container output and are no longer appended
  to `/var/log/datadog/init.log`.

## Rolling back to s6

For this release, set `ENTRYPOINT=s6-all-in-one`. It is an environment variable read by the
image, not Docker's `--entrypoint`. Do not override the container entrypoint or command.

- Docker: `docker run -e ENTRYPOINT=s6-all-in-one ...`
- Compose: `environment: [ENTRYPOINT=s6-all-in-one]`
- ECS: `{"name": "ENTRYPOINT", "value": "s6-all-in-one"}` in the Agent container definition
- Kubernetes: `env: [{name: ENTRYPOINT, value: s6-all-in-one}]`, or the profile `env` override
  for the Agent sidecar

Images older than the release that added `s6-all-in-one` exit 1 with
`"s6-all-in-one" is not a valid ENTRYPOINT.` when the variable is set. Remove it before
downgrading. s6 will be removed in a future release.
