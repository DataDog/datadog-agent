# GPU hardware integration review guidelines

Scope: this directory's tests that run against real GPUs and drivers.

## Real-device timing

This section narrows the root recommendation to avoid time-dependent tests. Do not request an injected clock or synctest to replace elapsed time required for real GPU work or driver sampling. Bounded wall-clock collection intervals are valid for those measurements. Check workload readiness, explicit deadlines, and process cleanup. Continue to flag arbitrary readiness sleeps; use deterministic synchronization for in-process timing logic that can be isolated.

## Device identity across APIs

When a test selects devices through CUDA and checks expectations through NVML, use UUIDs or explicitly align enumeration order. Flag ordinal-to-UUID assumptions that only happen to work on a single-GPU host. Make hardware-dependent skips visible; a single-GPU run does not validate multi-device mapping.
