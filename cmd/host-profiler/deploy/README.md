# Deploying Datadog's Host Profiler

The Host Profiler collects CPU and memory profiles at the OS level across all processes, regardless of language or runtime.

All commands in these docs assume you are running from this directory:

```shell
cd cmd/host-profiler/deploy
```

**If the Datadog Agent is already deployed on your cluster**, use **[Bundled](bundled/README.md)** mode. The host profiler runs as a sidecar and the Agent enriches profiles.

**Otherwise**, use **[Standalone](standalone/README.md)** mode. The host profiler runs independently with no Agent required.

If something isn't working, see [Troubleshooting](troubleshooting.md).

## Requirements

**OS:** Linux (kernel 5.10+)

**Architecture:** amd64, arm64

## Common usage

### Service naming

The host profiler determines each process's service name from its `OTEL_SERVICE_NAME` environment variable.

If `OTEL_SERVICE_NAME` is not set, the profiler infers the service name from the binary name. For interpreted languages, this is the interpreter name (for example, `java` for a Java process). If multiple services share the same interpreter and none set `OTEL_SERVICE_NAME`, their profiles are grouped under the same inferred name.

Set `OTEL_SERVICE_NAME` on each workload to identify them separately:

```yaml
env:
  - name: OTEL_SERVICE_NAME
    value: my-service
```

#### Datadog-specific naming

The profiler is also compatible with `DD_SERVICE` as replacement for `OTEL_SERVICE_NAME`.

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

## Security

The host profiler does not run privileged. It requests only the specific Linux capabilities it needs (`BPF`, `PERFMON`, `SYS_PTRACE`, `SYS_RESOURCE`, `DAC_READ_SEARCH`, `SYSLOG`, `CHECKPOINT_RESTORE`, `IPC_LOCK`).

**seccomp** further restricts the syscalls the container can make beyond what those capabilities allow. The profile ships at `/etc/dd-host-profiler/seccomp.json` inside the image and is applied automatically in all deployment paths except the Datadog Operator, which requires manual provisioning to each node.

**AppArmor** is optional but recommended where available. It restricts which binaries the profiler can exec: only `objcopy`, used for symbol extraction, is permitted.
