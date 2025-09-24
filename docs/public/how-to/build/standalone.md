# How to build standalone agent binaries

-----

## Building agent binaries

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

/// info | Other Agent binaries

Other agent binaries are built using `dda inv <target>.build` commands. Some examples are:

```
dda inv dogstatsd.build
dda inv otel-agent.build
dda inv system-probe.build
dda inv trace-agent.build
```

You can find the full list of buildable agent-related binaries [here](../../reference/builds/components.md#agent-binaries).
///

### Including or excluding Agent features

Different features of the Agent can be included / excluded at build time, by leveraging [Go build constraints](https://pkg.go.dev/cmd/go#hdr-Build_constraints). This can be done by passing the `--build-include` or `--build-exclude` flags to the build commands. A (non-exhaustive) list of available features can be found [here](../../reference/builds/components.md#agent-features).

The set of features enabled by default (i.e. with no flag) depends on the build context: which binary you are trying to build, which flavor of the agent, which platform you are building on etc.

/// info
If you want to replicate the same configuration of the Agent as the one distributed in system packages, you need to use this default set of features - so **no flag needs to be passed**.
///

/// details | Determining the default set of features
    open: False
    type: tip

The default set of features is determined by the [`get_default_build_tags` method](https://github.com/DataDog/datadog-agent/blob/main/tasks/build_tags.py#L394).

There is a command you can use to print out the default build tags for your build context:
```bash
dda inv print-default-build-tags
```

You can give more info about your build context using the `-b`, `-f` and `-p` flags:
```bash
dda inv print-default-build-tags -b otel-agent -p windows
> otlp,zlib,zstd
dda inv print-default-build-tags -f fips
> bundle_installer,consul,datadog.no_waf,ec2,etcd,fargateprocess,goexperiment.systemcrypto,grpcnotrace,jmx,kubeapiserver,kubelet,ncm,oracle,orchestrator,otlp,python,requirefips,trivy_no_javadb,zk,zlib,zstd
```
Run `dda inv print-default-build-tags --help` for more details.
///

/// example
To include the `zstd`, `etcd` and `python` features:
```bash
dda inv <target>.build --build-include=zstd,etcd,python
```

To exclude some features that would otherwise be enabled:
```bash
dda inv <target>.build --build-exclude=systemd,python
```
///

## Running agents

You can run the Core Agent directly in the foreground with the following command.

```
./bin/agent/agent run -c bin/agent/dist/datadog.yaml
```

/// note
The file `bin/agent/dist/datadog.yaml` is copied from `dev/dist/datadog.yaml` by `dda inv agent.build` and must contain a valid API key. If this did not already exist, you can create a file at any path and reference that with the `-c` flag instead.
///

## Agent Bundles
As an option, the Agent can combine functionality from multiple binaries into a single one to reduce the space used on disk. We call this a "bundled agent".


### Building an agent bundle

To build a bundled agent, simply use the `--bundle` flag with the `dda inv agent.build` to include the features from other binaries alongside the main `agent` into your final artifacts.

/// example
To create a binary that contains the features from the main `agent`, as well as the features from `process-agent` and `security-agent`, use:
```bash
dda inv agent.build --bundle process-agent --bundle security-agent
```
///

// details | Under the hood
    open: False
    type: info

Making a bundle - combining functionality from multiple binaries - just corresponds to building an agent binary including the source code from the others.

Like other features, this is accomplished through Go build constraints. Under the hood, building with a `--bundle` argument simply corresponds to including a special agent "feature".
> Those special features are named in a predictable pattern: `bundle_<binary name>`, ex: `bundle_process_agent`.

Thus, the two following commands are equivalent:
```bash
dda inv agent.build --bundle process-agent --bundle security-agent
dda inv agent.build --build-include=bundle_process_agent,bundle_security_agent
```
///

### Using an agent bundle

The bundled agent binary, when executed, will dynamically determine which binary to act as. This is determined according to:

1. The value of the `DD_BUNDLED_AGENT` environment variable.
1. If it is not set, the process name is used instead.
1. As a fallback, the executable will behave as the 'main' Agent.


/// example
```bash
# Build the agent bundle
dda inv agent.build --bundle process-agent
# -- The built artifact is available in bin/agent/agent

# This behaves as the main agent
./bin/agent/agent

# This behaves as the process-agent
DD_BUNDLED_AGENT=process-agent ./bin/agent/agent
```
///
