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

The image starts four services:

- `agent` is the main agent. The container will exit if it stops.
- `trace-agent`, `process-agent`, and `system-probe` are auxiliary services.
They will be restarted after crashing, but not if exiting normally (for example, the
`trace-agent` will disable itself if `DD_APM_ENABLED` is false).

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
