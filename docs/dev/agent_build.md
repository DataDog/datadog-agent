# Building the Agent

## Be modular

You can decide at build time which components of the Agent you want to find in
the final artifact. By default, all the components are picked up, so if you want
to replicate the same configuration of the Agent distributed via system packages,
all you have to do is `dda inv agent.build`.

To pick only certain components you have to invoke the task like this:

```
dda inv agent.build --build-include=zstd,etcd,python
```

Conversely, if you want to exclude something:

```
dda inv agent.build --build-exclude=systemd,python
```

## Trace agent


```
dda inv trace-agent.build
```
To run the trace agent, with the Datadog Agent already running, run `./bin/trace-agent/trace-agent`. If needed, pass your configuration file with `-config <path-to-yaml-file>`.

## Additional details

We use `pkg-config` to make compilers and linkers aware of Python. The required .pc files are
provided automatically when building python through omnibus.

As an option, the Agent can combine multiple functionalities into a single binary to reduce
the space used on disk. The `DD_BUNDLED_AGENT` environment variable is used to select
which functionality to enable. For instance, if set to `process-agent`, it will act as the process Agent.
If the environment variable is not defined, the process name is used as a fallback.
As the last resort meaning, the executable will behave as the 'main' Agent.

Different combinations can be obtained through the usage of build tags. As an example,
building the Agent with the `bundle_process_agent` and `bundle_security_agent` will produce
a binary that has the process Agent and security Agent capabilities.

The `--bundle` argument can be used to override the default set of functionalities bundled
into the Agent binary. For instance, to override the defaults and bundle only the process and
and the security Agents:

```
dda inv agent.build --bundle process-agent --bundle security-agent
```

To disable bundling entirely:

```
dda inv agent.build --bundle agent
```

## Testing Agent changes in containerized environments

Building an Agent Docker image from scratch through an embedded build is a slow process.
You can quickly test a change or bug fix in a containerized environment (such as Docker, Kubernetes, or ECS).

One way to do this is to patch the Agent binary from an official Docker image, with a Dockerfile:

```
FROM datadog/agent:<AGENT_VERSION>

COPY agent /opt/datadog-agent/bin/agent/agent
```

For this to work properly, two things are important:
- Your change needs to be done on top of the `<AGENT_VERSION>` tag from the DataDog repository.
- You need to run the invoke task with the proper embedded path `dda inv -e agent.build -e /opt/datadog-agent/embedded`.

**Note**: This makes `invoke` install the build's artifacts in the `/opt/datadog-agent/embedded` folder. Make sure the folder exists and the current user has write permissions.
