# Datadog Trace Agent

An agent that collects traces from various sources, normalizes, pre-processes, samples them and computes
statistics before sending them to the Datadog API.

## Run on Linux or macOS

The Trace Agent is packaged with the standard Datadog Agent.
Just [run the Datadog Agent](http://docs.datadoghq.com/guides/basic_agent_usage/).

To install the trace agent from source, follow the instructions in the [development](#development)
section of this document.

## Run on Windows

On Windows, the trace agent is shipped together with the Datadog Agent only
since version 5.19.0, so users must update to 5.19.0 or above. However the
Windows trace agent is in beta and some manual steps are required.

Update your config file to include:

```
[Main]
apm_enabled: yes
[trace.config]
log_file = C:\ProgramData\Datadog\logs\trace-agent.log
```

Restart the datadogagent service:

```
net stop datadogagent
net start datadogagent
```

For this beta the trace agent status and logs are not displayed in the Agent
Manager GUI.

To see the trace agent status either use the Service tab of the Task Manager or
run:

```
sc.exe query datadog-trace-agent
```

And check that the status is "running".

## Development

First, make sure Go 1.11+ is installed. You can do this by following the steps on the [official website](https://golang.org/dl/).
After cloning the repo, simply run the following command in the root of the `datadog-agent` repository:

```bash
go install ./cmd/trace-agent
```

You may now run the agent using `trace-agent` (considering the path `$GOPATH/bin` is in your system's `$PATH`). For any type
of troubleshooting, check the agent output or logs (`/var/log/datadog/trace-agent.log` on Linux) to ensure that traces are sane
and that they are reaching the Datadog API.
