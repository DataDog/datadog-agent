# How to build standalone agent binaries

-----

## Core Agent

The Core Agent is built using the `dda inv agent.build` command.

```
dda inv agent.build --build-exclude=systemd
```

Running this command will:

- Discard any changes done in `bin/agent/dist`.
- Build the Agent and write the binary to `bin/agent/agent`, with a `.exe` extension on Windows.
- Copy files from [`dev/dist`](https://github.com/DataDog/datadog-agent/blob/main/dev/dist/README.md) to `bin/agent/dist`.

/// note | Caveat
If you built an older version of the Agent and are encountering the error `make: *** No targets specified and no makefile found`, remove the `rtloader/CMakeCache.txt` file.
///

## Other agent flavors

Other agent flavors are built using `dda inv <flavor>.build` commands. Some examples are:

```
dda inv dogstatsd.build
dda inv otel-agent.build
dda inv system-probe.build
dda inv trace-agent.build
```

## Running agents

You can run the Core Agent directly in the foreground with the following command.

```
./bin/agent/agent run -c bin/agent/dist/datadog.yaml
```

/// note
The file `bin/agent/dist/datadog.yaml` is copied from `dev/dist/datadog.yaml` by `dda inv agent.build` and must contain a valid API key. If this did not already exist, you can create a file at any path and reference that with the `-c` flag instead.
///
