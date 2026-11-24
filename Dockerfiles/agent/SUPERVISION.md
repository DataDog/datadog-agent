# Embedded supervisor for the agent6 image

The Datadog Agent is currently split in four binaries running cooperatively:

  - the main `agent`, collecting metrics, events and logs
  - the `trace-agent`, collecting APM traces
  - the `process-agent`, collecting live container and process data
  - the `system-probe`, collecting network data, accessible by the process-agent

In order to provide an all-in-one image, we are including a process supervisor.
We are using [`s6`](https://skarnet.org/software/s6/s6-svc.html) via the
[s6-overlay](https://github.com/just-containers/s6-overlay) project as:

- it is light and customisable
- it correctly handles signals and process reaping
- it has been battle tested in docker containers since 2015

## Entrypoint scripts

The entrypoint has been split up in several scripts in the `/etc/cont-init.d/` folder.
They will be run in alphabetical order by s6 during the container startup.
The `90-` to `99-` prefixes are available for injecting custom scripts, to be executed
after the provided ones.

You can use custom scripts to:

- `pip install` python dependencies to your custom checks
- append content at the end of `/etc/datadog-agent/datadog.yaml`
- disable standard checks by editing the files in `/etc/datadog-agent/conf.d/`

**Note:** environment variables exported in these scripts will not propagate to the agent.
The supported way to pass envvars to the agent is to set container envvars.

## Services

The image starts the following services:

- `agent` is the main agent. The container will exit if it stops.
- `trace-agent`, `process-agent`, `system-probe`, and `data-plane` (Agent Data Plane)
are auxiliary services. They will be restarted after crashing, but not if exiting
normally (for example, the `trace-agent` will disable itself if `DD_APM_ENABLED` is
false). The `data-plane` service self-disables if the `agent-data-plane` binary is
not bundled in the image.

## Useful commands

Each agent runs as a separate service. You can use the
[`s6-svstat`](https://skarnet.org/software/s6/s6-svstat.html) and
[`s6-svc`](https://skarnet.org/software/s6/s6-svc.html)
commands to manage them:

#### Get the process-agent status
```
root@95c063fac5c4:/# s6-svstat /var/run/s6/services/process/
up (pid 2219) 39 seconds
```

#### Force kill it, it will be restarted after a short pause
```
root@95c063fac5c4:/# kill -9 $(pidof process-agent)
root@95c063fac5c4:/# s6-svstat /var/run/s6/services/process/
up (pid 2449) 3 seconds
```

#### Disable the process-agent
```
root@95c063fac5c4:/# s6-svc -d /var/run/s6/services/process/
root@95c063fac5c4:/# s6-svstat /var/run/s6/services/process/
down (exitcode 0) 5 seconds, normally up, ready 5 seconds
```

#### Start it back
```
root@95c063fac5c4:/# s6-svc -u /var/run/s6/services/process/
root@95c063fac5c4:/# s6-svstat /var/run/s6/services/process/
up (pid 2597) 5 seconds
```

**Note:** if you need to restart the main agent (for debugging purposes), you need to first remove
the `/var/run/s6/services/agent/finish` file, to avoid it bringing down the container on exit. You
can then run `kill $(pidof agent)` to trigger a restart of the agent.

## Opt-in dd-procmgrd supervision

Setting the container environment variable `ENTRYPOINT=procmgr-all-in-one` runs the processes
under `dd-procmgrd`, the Datadog process manager, instead of s6. It is an environment variable
read by the image, not Docker's `--entrypoint`.

`procmgr-all-in-one` runs the cont-init scripts, then execs `dd-procmgrd`. When there are
custom services or shipped `down` files, it first stages a copy of the catalog and the
custom services. Otherwise the daemon reads the catalog in the image. The daemon is PID 1,
or the main process when the container uses `pid: host`.

### Processes

Each process starts only when its binary is in the image and its configuration gate is open.
A closed gate shows as `Skipped` with the reason.

| Process | Binary | Starts when |
|---------|--------|-------------|
| `datadog-agent` | `agent` | Always. It is critical: when it exits, the container exits |
| `datadog-agent-trace` | `trace-loader`, then `trace-agent` | `apm_config.enabled` or `apm_config.error_tracking_standalone.enabled` |
| `datadog-agent-process` | `process-agent` | Any `process_config` collection setting, or `network_config.enabled` or `system_probe_config.enabled` in `system-probe.yaml` |
| `datadog-agent-sysprobe` | `system-probe` | `network_config.enabled`, `system_probe_config.enabled` or `runtime_security_config.enabled` in `system-probe.yaml`, or `software_inventory.enabled`, unless `system_probe_config.external` is set |
| `datadog-agent-security` | `security-agent` | `compliance_config.enabled`, or `runtime_security_config.enabled` in `system-probe.yaml` |
| `datadog-agent-otel` | `otel-agent` | `otelcollector.enabled` |
| `datadog-agent-data-plane` | `agent-data-plane` | `data_plane.enabled` |
| `datadog-agent-par` | `privateactionrunner` | `private_action_runner.enabled` |
| `datadog-agent-par-control` | `par-control` | `private_action_runner.split_enabled`, unless `fips.enabled` is set |
| `datadog-agent-action-executor` | `privateactionrunner` | Never at boot. `par-control` starts it on demand |

Auxiliary processes restart after a non-zero exit or a signal. The first wait is 2 seconds.
The delay doubles up to 60 seconds while the process exits within 1 second of starting, and
returns to 2 seconds after a run of at least 1 second. A process that exits 0 stays stopped.
`datadog-agent` and `datadog-agent-action-executor` never restart.

Every process inherits the container `PATH`. `DD_PM_*` variables are not passed to the
processes, except `DD_PM_SOCKET_PATH`.

### Cont-init scripts

Before `dd-procmgrd` starts, the entrypoint copies each regular file in `/etc/cont-init.d/`
(C-locale order) to a staging directory, makes the copies executable, and runs them. The
staging directory is the first of `/var/run/s6`, `${TMPDIR:-/tmp}`, or `/opt/datadog-agent/run`
where a probe script can execute. A temporary staging directory is removed after cont-init.

- Start each script with a shebang, because it runs directly. A failing script stops the
  container before `dd-procmgrd` starts, with the script's exit code.
- Scripts run with `S6_KEEP_ENV=1`, so a `#!/usr/bin/with-contenv` shebang sees the container
  environment even with `S6_KEEP_ENV=0`.
- Scripts run in their own session and process group with SIGINT and SIGQUIT ignored.
  Stopping the container during cont-init sends SIGTERM to that process group, including
  children it forked, and the entrypoint waits for them to exit. A child that moves to its own
  process group is not signaled. A script that ignores SIGTERM gets SIGKILL 5 seconds
  later, so the container exits with code 143 before Docker's default 10-second timeout.
  A `docker stop -t` under 5 seconds ends the container first, with code 137.

### Custom services

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
Edits to a shipped service's `run` file have no effect.

A staged copy under `/opt/datadog-agent/run` (a VOLUME) is removed on a graceful stop. If
`dd-procmgrd` is killed first, the directory stays until the volume is removed.

### Useful commands

`dd-procmgr` finds the supervisor socket on its own, so plain `docker exec` works:

```
docker exec <container> dd-procmgr list
docker exec <container> dd-procmgr stop datadog-agent-process
docker exec <container> dd-procmgr start datadog-agent-process
```

To restart a process, run `stop` then `start`. Stopping `datadog-agent` exits the container
with code 0, so restart the container instead.

### Exit codes and stopping

- When the core Agent exits, the other processes stop and the container exits with the Agent's
  exit code, or 128 plus the signal number when a signal killed it (137 after `SIGKILL`).
- On `docker stop`, each process gets its `stop_timeout` (90 seconds by default, 180 seconds
  for `datadog-agent-par-control`) before it is force-killed. A clean shutdown exits 0. If
  processes are still running when Docker's default 10-second grace period ends, the container
  exits 137.
- `docker stop` during cont-init exits 143. An interrupt (INT) before `dd-procmgrd` starts
  also exits 143.

### Differences from s6

- `finish` scripts, and scripts in `/etc/cont-finish.d/` and `/etc/fix-attrs.d/`, do not run.
- `S6_*` variables do not control supervision. `s6-svc` and `s6-svstat` do not control
  `dd-procmgrd`. `dd-procmgr` replaces them.
- Start, exit, and restart messages stay in the container output and are no longer appended
  to `/var/log/datadog/init.log`.
