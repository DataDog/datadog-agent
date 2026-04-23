# Fake NVML Library (`pkg/gpu/fake-nvml`)

A Rust `cdylib` that implements the [NVML](https://docs.nvidia.com/deploy/nvml-api/) C ABI. It returns synthesized data for a configurable GPU architecture (Hopper by default) and lets you run the Datadog Agent's GPU check — and observe its full metric pipeline — on any Linux machine without real NVIDIA hardware.

The set of supported architectures and their representative device values is driven by [`pkg/collector/corechecks/gpu/spec/architectures.yaml`](../../collector/corechecks/gpu/spec/architectures.yaml), the same file the agent's GPU check consumes. Select an architecture at process start via the `FAKE_NVML_ARCH` environment variable.

## Quick start

```bash
# 1. Build the fake library and the agent
bazelisk build //pkg/gpu/fake-nvml:fake_nvml
dda inv agent.build --build-exclude=systemd

# 2. Write config (AFTER the build — the build overwrites bin/agent/dist/datadog.yaml)
cat > bin/agent/dist/datadog.yaml <<EOF
api_key: "0000001"
hostname: "fake-gpu-test"
gpu:
  enabled: true
  nvml_lib_path: "$(pwd)/bazel-bin/pkg/gpu/fake-nvml/libfake_nvml.so"
EOF

# 3. Run a one-shot check to see all 44 metric series
./bin/agent/agent check gpu -c bin/agent/dist/datadog.yaml
```

That's it — you'll see JSON output with 22 distinct metrics × 2 fake devices (44 series total), including `gpu.temperature`, `gpu.memory.free`, `gpu.power.usage`, `gpu.clock.speed.*`, etc.

To run the agent as a daemon instead:

```bash
# Optional: set 1s collection interval for faster iteration
mkdir -p bin/agent/dist/conf.d/gpu.d
cat > bin/agent/dist/conf.d/gpu.d/conf.yaml <<EOF
instances:
  - min_collection_interval: 1
EOF

./bin/agent/agent run -c bin/agent/dist/datadog.yaml
```

You should see log lines like:

```
INFO | Agent found NVML library at /…/bazel-bin/pkg/gpu/fake-nvml/libfake_nvml.so
INFO | Scheduling check gpu with an interval of 1s
INFO | check:gpu | Running check...
INFO | check:gpu | Done running check
```

WARN lines about unsupported collectors (`gpm`, `fields`, `nvlink`, `device_events`, `sampling`) are expected — those require hardware-specific NVML APIs the fake library intentionally omits.

## What it is

The library is built by Bazel as `libfake_nvml.so` and installed into the agent package at:

```
<install_dir>/embedded/dev/libnvidia-ml-fake.so.1
```

It is **never loaded automatically**. The agent's default NVML discovery only looks at:

```
/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1
/run/nvidia/driver/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1
```

The fake library has a different name (`libnvidia-ml-fake.so.1`) and lives in a non-standard path (`embedded/dev/`), so it cannot be picked up unless you explicitly configure it.

## Activation

On an installed agent:

```yaml
gpu:
  enabled: true
  nvml_lib_path: "/opt/datadog-agent/embedded/dev/libnvidia-ml-fake.so.1"
```

From a local build (point to the Bazel output directly):

```yaml
gpu:
  enabled: true
  nvml_lib_path: "<repo_root>/bazel-bin/pkg/gpu/fake-nvml/libfake_nvml.so"
```

Select a non-default architecture by exporting `FAKE_NVML_ARCH` in the agent's environment, e.g.:

```bash
FAKE_NVML_ARCH=ampere FAKE_NVML_DEVICE_COUNT=4 \
  ./bin/agent/agent run -c bin/agent/dist/datadog.yaml
```

## Fake device data

All exported devices share the architecture selected via `FAKE_NVML_ARCH` (homogeneous node). Identity fields come from the spec's `defaults:` block for that architecture:

| `FAKE_NVML_ARCH` | Name | CUDA CC | Cores | Memory | NVML arch |
|---|---|---|---|---|---|
| `pascal`             | Tesla P40             | 6.1  | 3840  | 24 GiB  | 4 |
| `volta`              | Tesla V100-SXM2-32GB  | 7.0  | 5120  | 32 GiB  | 5 |
| `turing`             | Tesla T4              | 7.5  | 2560  | 15 GiB  | 6 |
| `ampere`             | A100-SXM4-80GB        | 8.0  | 6912  | 80 GiB  | 7 |
| `hopper` *(default)* | H100 80GB HBM3        | 9.0  | 16384 | 80 GiB  | 9 |
| `ada`                | L40S                  | 8.9  | 18176 | 45 GiB  | 8 |
| `blackwell`          | B200                  | 10.0 | 16896 | 192 GiB | 10 |

Runtime knobs (read at process start):

| Env var | Default | Meaning |
|---|---|---|
| `FAKE_NVML_ARCH`         | `hopper` | Architecture name (must match a key in `architectures.yaml`). Unknown values log a warning to stderr and fall back to `hopper`. |
| `FAKE_NVML_DEVICE_COUNT` | `2`      | Number of fake devices to expose, clamped to 1..=16. |

Secondary fields (temperature, power, clocks, fake PID) are plausible
architecture-agnostic constants — enough to exercise every stateless
collector in the agent, but not intended to model a specific SKU exactly.
Metrics vary slightly per device (e.g. temperature, fan speed, energy) so
scraped series are distinguishable.

## Metrics emitted

`agent check gpu` produces 22 distinct metrics (44 series across both devices):

```
gpu.clock.speed.graphics       gpu.clock.speed.graphics.max
gpu.clock.speed.memory         gpu.clock.speed.memory.max
gpu.clock.speed.sm             gpu.clock.speed.sm.max
gpu.clock.speed.video          gpu.clock.speed.video.max
gpu.device.total               gpu.fan_speed
gpu.memory.bar1.free           gpu.memory.bar1.total
gpu.memory.bar1.used           gpu.memory.free
gpu.memory.limit               gpu.memory.reserved
gpu.performance_state           gpu.power.management_limit
gpu.power.usage                gpu.process.memory.usage
gpu.temperature                gpu.total_energy_consumption
```

Collectors that require hardware-specific features degrade gracefully:

| Collector | Status |
|---|---|
| `stateless` (memory, clocks, power, temperature) | ✅ Fully active |
| `sampling` (process utilization) | ❌ Disabled — NVML samples API not implemented |
| `gpm` (tensor/fp16/fp32 active) | ❌ Disabled — GPM not implemented |
| `fields` (NVLink, C2C) | ❌ Disabled — `nvmlDeviceGetFieldValues` not implemented |
| `device_events` (XID errors) | ❌ Disabled — event set API not implemented |
| `nvlink` (PLR counters) | ❌ Disabled — PRM TLV API not implemented |
| `ebpf` (system-probe) | ⚠️ Only active if system-probe is also running |

## Building

```bash
# Debug build
bazelisk build //pkg/gpu/fake-nvml:fake_nvml

# Release build (LTO + size optimization)
bazelisk build --config=release //pkg/gpu/fake-nvml:fake_nvml

# Verify exported symbols (24 nvml* symbols)
nm -D bazel-bin/pkg/gpu/fake-nvml/libfake_nvml.so | grep nvml
```

## Security note

This library is a development tool. Do not deploy it on production hosts. The `embedded/dev/` path and the non-standard filename make accidental activation difficult, but explicit configuration is still possible. Treat it like any other development binary.
