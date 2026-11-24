// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

/*
Package integrationtests contains integration tests for the GPU core check
that require real GPU hardware to run. These tests are separated from the unit
tests in the parent package to allow running them on dedicated GPU runners.

These tests validate the GPU check's interaction with real NVML library and
GPU hardware, including metrics collection and device enumeration.

These tests are NOT run as part of the regular CI pipeline on main branch.
They run:
  - On nightly pipelines
  - On feature branches when GPU-related code changes
  - Manually when triggered

To run these tests locally, you need a machine with:
  - A supported NVIDIA GPU
  - NVIDIA drivers installed
  - NVML library available

TestSpecMIG validates GPUs in MIG mode. It runs on a host that is already in MIG mode, or, with
GPU_TEST_CONFIGURE_MIG=1 (any boolean true), it enables MIG mode on the MIG-capable GPUs and creates MIG devices through
NVML, then restores the GPUs when it ends. That needs root and GPUs that are not in use.

By default the check runs without system-probe, so the metrics that only the system-probe GPU module
provides are not validated. With GPU_TEST_SYSTEM_PROBE_BIN set to a system-probe binary, TestSpec and
TestSpecMIG start it with the GPU module and its eBPF probes, as it runs in production, point the
check at it, and also validate the metrics that need it. That needs root, and the eBPF objects
installed root-owned in /opt/datadog-agent/embedded/share/system-probe/ebpf (or in the directory
named by GPU_TEST_SYSTEM_PROBE_BPF_DIR). `dda inv system-probe.build` builds both.

When a test fails, the system-probe output and logs are kept in a gpu-spec-system-probe-* directory
under /tmp, and the test log names it.

Run it while no other system-probe with the GPU eBPF probes is running on the host.
*/
package integrationtests
