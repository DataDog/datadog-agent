# Fake NVML demo library

Minimal Rust `cdylib` used by the GPU job metadata demo to run the real `gpu`
core check on Linux without NVIDIA hardware.

Build:

```bash
bazelisk build //pkg/gpu/fake-nvml:fake_nvml
```

Run the Agent with:

```yaml
gpu:
  enabled: true
  nvml_lib_path: "<repo>/bazel-bin/pkg/gpu/fake-nvml/libfake_nvml.so"
```

Useful demo env vars:

- `FAKE_NVML_DEVICE_COUNT=1` — expose one fake GPU.
- `FAKE_NVML_PROCESS_PID=<host pid>` — make fake process metrics use a real
  process PID, typically a demo container's init PID, so the existing GPU
  workload tag path can resolve container/job tags.

This is a development/demo binary only; it is not installed or auto-loaded.
