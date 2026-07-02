"""Canonical eBPF program target lists, shared by Python and Bazel.

This is the single source of truth for the Bazel targets that produce the
compiled eBPF .o programs consumed via DD_SYSTEM_PROBE_BPF_DIR (prebuilt
programs go to build_dir/, CO-RE programs go to build_dir/co-re/). It is
deliberately written in the common subset of Starlark and Python so that:

  - Bazel BUILD files can `load()` PREBUILT_TARGETS / CORE_TARGETS directly, and
  - tasks/system_probe.py can exec it to obtain the same data.
"""

PREBUILT_TARGETS = [
    "//pkg/network/ebpf/c/prebuilt:dns",
    "//pkg/network/ebpf/c/prebuilt:dns-debug",
    "//pkg/network/ebpf/c/prebuilt:offset-guess",
    "//pkg/network/ebpf/c/prebuilt:offset-guess-debug",
    "//pkg/network/ebpf/c/prebuilt:tracer",
    "//pkg/network/ebpf/c/prebuilt:tracer-debug",
    "//pkg/network/ebpf/c/prebuilt:usm",
    "//pkg/network/ebpf/c/prebuilt:usm-debug",
    "//pkg/network/ebpf/c/prebuilt:usm_events_test",
    "//pkg/network/ebpf/c/prebuilt:usm_events_test-debug",
    "//pkg/network/ebpf/c/prebuilt:shared-libraries",
    "//pkg/network/ebpf/c/prebuilt:shared-libraries-debug",
    "//pkg/network/ebpf/c/prebuilt:conntrack",
    "//pkg/network/ebpf/c/prebuilt:conntrack-debug",
    "//pkg/security/ebpf/c/prebuilt:runtime-security",
    "//pkg/security/ebpf/c/prebuilt:runtime-security-syscall-wrapper",
    "//pkg/security/ebpf/c/prebuilt:runtime-security-fentry",
    "//pkg/security/ebpf/c/prebuilt:runtime-security-offset-guesser",
]

CORE_TARGETS = [
    "//pkg/ebpf/c:lock_contention",
    "//pkg/ebpf/c:ksyms_iter",
    "//pkg/network/ebpf/c:tracer",
    "//pkg/network/ebpf/c/sk:sk_tracer",
    "//pkg/network/ebpf/c/sk:sk_tracer-debug",
    "//pkg/network/ebpf/c:tracer-debug",
    "//pkg/network/ebpf/c/co-re:tracer-fentry",
    "//pkg/network/ebpf/c/co-re:tracer-fentry-debug",
    "//pkg/network/ebpf/c/runtime:usm",
    "//pkg/network/ebpf/c/runtime:usm-debug",
    "//pkg/network/ebpf/c/runtime:shared-libraries",
    "//pkg/network/ebpf/c/runtime:shared-libraries-debug",
    "//pkg/network/ebpf/c/runtime:conntrack",
    "//pkg/network/ebpf/c/runtime:conntrack-debug",
    "//pkg/collector/corechecks/ebpf/c/runtime:oom-kill",
    "//pkg/collector/corechecks/ebpf/c/runtime:oom-kill-debug",
    "//pkg/collector/corechecks/ebpf/c/runtime:tcp-queue-length",
    "//pkg/collector/corechecks/ebpf/c/runtime:tcp-queue-length-debug",
    "//pkg/collector/corechecks/ebpf/c/runtime:ebpf",
    "//pkg/collector/corechecks/ebpf/c/runtime:ebpf-debug",
    "//pkg/collector/corechecks/ebpf/c/runtime:noisy-neighbor",
    "//pkg/collector/corechecks/ebpf/c/runtime:noisy-neighbor-debug",
    "//pkg/gpu/ebpf/c/runtime:gpu",
    "//pkg/gpu/ebpf/c/runtime:gpu-debug",
    "//pkg/dyninst/ebpf:dyninst_event",
    "//pkg/dyninst/ebpf:dyninst_event-debug",
    "//pkg/ebpf/testdata/c:logdebug-test",
    "//pkg/ebpf/testdata/c:error_telemetry",
    "//pkg/ebpf/testdata/c:sleepable",
    "//pkg/ebpf/testdata/c:uprobe_attacher-test",
    "//cmd/system-probe/subcommands/ebpf/testdata:btf_test",
]
