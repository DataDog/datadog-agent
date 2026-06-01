# Deploying Datadog's Host Profiler

The Host Profiler collects CPU and memory profiles at the OS level across all processes, regardless of language or runtime.

All commands in these docs assume you are running from this directory:

```shell
cd cmd/host-profiler/deploy
```

**If the Datadog Agent is already deployed on your cluster**, use **[Bundled](bundled/README.md)** mode. The host profiler runs as a sidecar and the Agent forwards profiles to Datadog.

**Otherwise**, use **[Standalone](standalone/README.md)** mode. The host profiler runs independently with no Agent required.

If something isn't working, see [Troubleshooting](troubleshooting.md).

## Requirements

**OS:** Linux (kernel 5.10+)

**Architecture:** amd64, arm64

## Common usage

### Service naming

The host profiler determines each process's service name from its `DD_SERVICE` environment variable.

If `DD_SERVICE` is not set, the profiler infers the service name from the binary name. For interpreted languages, this is the interpreter name (for example, `java` for a Java process). If multiple services share the same interpreter and none set `DD_SERVICE`, their profiles are grouped under the same inferred name.

Set `DD_SERVICE` on each workload to identify them separately:

```yaml
env:
  - name: DD_SERVICE
    value: my-service
```

Set `DD_ENV` and `DD_VERSION` for richer filtering in the Profiler UI.

### Manually uploading debug symbols

For compiled languages (C, C++, Rust, Go), the host profiler uploads debug symbols to Datadog for symbolization. Binaries must include debug symbols (not stripped) for function names to appear in profiles.

To upload symbols from stripped binaries:

1. Install the [datadog-ci CLI](https://github.com/DataDog/datadog-ci).
2. Set your API key and site:

```shell
export DD_API_KEY=<DATADOG_API_KEY>
export DD_SITE=<DATADOG_SITE>
```

3. Upload symbols:

```shell
DD_BETA_COMMANDS_ENABLED=1 datadog-ci elf-symbols upload /path/to/build/symbols/
```
