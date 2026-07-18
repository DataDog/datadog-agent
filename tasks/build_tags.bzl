"""Canonical build-tag data, shared by Python and Bazel.

This is the single source of truth for the agent's Go build tags. It is
deliberately written in the common subset of Starlark and Python so that:

  - //BUILD.bazel can `load()` GAZELLE_BUILD_TAGS directly, and
  - tasks/build_tags.py can exec it to obtain the same data.

Keep it in that subset: build sets with `set([...])` (a `{...}` literal is a
dict in Starlark, not a set) and use set operators/methods (`|`, `-`,
`.union()`, `.difference()`). The AgentFlavor-keyed mapping and the codegen
payload live in build_tags.py, because Starlark has no enums.
"""

# Common build tags, added on all builds
COMMON_TAGS = set([
    # removes the import to golang.org/x/net/trace in google.golang.org/grpc,
    # which prevents dead code elimination, see https://github.com/golang/go/issues/62024
    "grpcnotrace",
    # removes the import to golang.org/x/net/trace in github.com/grpc-ecosystem/go-grpc-middleware
    # which prevents dead code elimination, see https://github.com/golang/go/issues/62024
    "retrynotrace",
    # Remove some dependencies from Trivy to reduce binary size.
    "trivy_no_javadb",
])

# ALL_TAGS lists all available build tags.
# Used to remove unknown tags from provided tag lists.
ALL_TAGS = set([
    "bundle_installer",
    "clusterchecks",
    "consul",
    "containerd",
    "cri",
    "crio",
    # Opt out of the ASM build requirements of dd-trace-go
    "datadog.no_waf",
    "docker",
    "ec2",
    "etcd",
    "fargateprocess",
    "goexperiment.systemcrypto",  # used for FIPS mode
    "jetson",
    "jmx",
    "kubeapiserver",
    "kubelet",
    "linux_bpf",
    "ncm",
    "netcgo",  # Force the use of the CGO resolver. This will also have the effect of making the binary non-static
    "netgo",
    "no_gogo",  # drops the gogo/protobuf compatibility shim in containerd/typeurl
    "npm",
    "nvml",  # used for the nvidia go-nvml library
    "oracle",
    "orchestrator",
    "osusergo",
    "otlp",
    "pcap",  # used by system-probe to compile packet filters using google/gopacket/pcap, which requires cgo to link libpcap
    "podman",
    "python",
    "requirefips",  # used for Linux FIPS mode to avoid having to set GOFIPS
    "seclmax",  # used for security agent/system-probe to compile the full feature set of secl
    "serverless",
    "sharedlibrarycheck",
    "systemd",
    "systemprobechecks",  # used to include system-probe based checks in the agent build
    "test",  # used for unit-tests
    "trivy",
    "vrl",  # used by the logs agent to compile/evaluate VRL processing rules via a cgo bridge to a Rust static library
    "wmi",
    "zk",
    "zlib",
    "zstd",
    "cel",
    "cws_instrumentation_injector_only",  # used for building cws-instrumentation with only the injector code
    "remove_all_sd",  # remove all discovery provider from prometheusreceiver components
]).union(COMMON_TAGS)

# Tags Gazelle needs to see in addition to ALL_TAGS so it can analyse test-only
# files gated by them. Kept separate because they're test-only and don't belong
# in ALL_TAGS (which is also used to validate user-provided tag lists).
GAZELLE_EXTRA_TAGS = set([
    "e2ecoverage",
    "e2eunit",
    "functionaltests",
    "manualtest",
    "private_runner_experimental",
])

# Tags in ALL_TAGS that we deliberately keep out of Gazelle's set, typically
# because they require cgo/native deps that Gazelle's static analysis can't
# resolve cleanly.
GAZELLE_OMIT_TAGS = set(["pcap", "remove_all_sd"])

# Build tags Gazelle considers when analysing tag-gated .go files. Loaded by the
# root BUILD.bazel as the `build_tags` attribute of //:gazelle, so it must be a
# sorted list of strings (deterministic, and the gazelle rule wants a list).
GAZELLE_BUILD_TAGS = sorted((ALL_TAGS - GAZELLE_OMIT_TAGS) | GAZELLE_EXTRA_TAGS)

### Tag inclusion lists

# AGENT_TAGS lists the tags needed when building the agent.
AGENT_TAGS = set([
    "consul",
    "containerd",
    "cri",
    "datadog.no_waf",
    "crio",
    "docker",
    "ec2",
    "etcd",
    "fargateprocess",
    "jetson",
    "jmx",
    "kubeapiserver",
    "kubelet",
    "ncm",
    "netcgo",
    "nvml",
    "oracle",
    "orchestrator",
    "otlp",
    "podman",
    "python",
    "sharedlibrarycheck",
    "systemd",
    "systemprobechecks",
    "trivy",
    "vrl",  # benchmark-branch-only: compiles the real VRL cgo engine instead of the stub, see ralph/alp-vrl-log-processing-updated
    "zk",
    "zlib",
    "zstd",
    "cel",
])

# AGENT_HEROKU_TAGS lists the tags for Heroku agent build
AGENT_HEROKU_TAGS = AGENT_TAGS.difference(
    set([
        "containerd",
        "cri",
        "crio",
        "docker",
        "ec2",
        "fargateprocess",
        "jetson",
        "kubeapiserver",
        "kubelet",
        "nvml",
        "oracle",
        "orchestrator",
        "podman",
        "systemd",
        "trivy",
        "cel",
    ]),
).union(
    set([
        "bundle_installer",
    ]),
)

FIPS_TAGS = set(["goexperiment.systemcrypto", "requirefips"])

# CLUSTER_AGENT_TAGS lists the tags needed when building the cluster-agent
CLUSTER_AGENT_TAGS = set([
    "clusterchecks",
    "datadog.no_waf",
    "kubeapiserver",
    "orchestrator",
    "zlib",
    "zstd",
    "ec2",
    "cel",
])

# CLUSTER_AGENT_CLOUDFOUNDRY_TAGS lists the tags needed when building the cloudfoundry cluster-agent
CLUSTER_AGENT_CLOUDFOUNDRY_TAGS = set(["clusterchecks", "cel"])

# DOGSTATSD_TAGS lists the tags needed when building dogstatsd.
# no_gogo drops the legacy gogo/protobuf compatibility shim in containerd/typeurl;
# the containerd metric types dogstatsd unmarshals (cgroups/v3, hcsshim stats) all
# use the modern google.golang.org/protobuf runtime, so the shim is dead weight.
DOGSTATSD_TAGS = set(["containerd", "docker", "kubelet", "no_gogo", "podman", "zlib", "zstd"])

# IOT_AGENT_TAGS lists the tags needed when building the IoT agent
IOT_AGENT_TAGS = set(["jetson", "systemd", "zlib", "zstd"])

# INSTALLER_TAGS lists the tags needed when building the installer
INSTALLER_TAGS = set(["ec2"])

# PROCESS_AGENT_TAGS lists the tags necessary to build the process-agent
PROCESS_AGENT_TAGS = set([
    "containerd",
    "cri",
    "crio",
    "datadog.no_waf",
    "ec2",
    "docker",
    "fargateprocess",
    "kubelet",
    "netcgo",
    "podman",
    "zlib",
    "zstd",
])

# PROCESS_AGENT_HEROKU_TAGS lists the tags necessary to build the process-agent for Heroku
PROCESS_AGENT_HEROKU_TAGS = set([
    "datadog.no_waf",
    "fargateprocess",
    "netcgo",
    "zlib",
    "zstd",
])

# SECURITY_AGENT_TAGS lists the tags necessary to build the security agent
SECURITY_AGENT_TAGS = set([
    "netcgo",
    "datadog.no_waf",
    "docker",
    "zlib",
    "zstd",
    "ec2",
])

# SBOMGEN_TAGS lists the tags necessary to build sbomgen
SBOMGEN_TAGS = set([
    "trivy",
    "containerd",
    "docker",
    "crio",
])

# SERVERLESS_TAGS lists the tags necessary to build serverless
SERVERLESS_TAGS = set(["serverless", "otlp"])

# SYSTEM_PROBE_TAGS lists the tags necessary to build system-probe
SYSTEM_PROBE_TAGS = set([
    "datadog.no_waf",
    "ec2",
    "linux_bpf",
    "netcgo",
    "npm",
    "nvml",
    "pcap",
    "zlib",
    "zstd",
    "seclmax",
])

# TRACE_AGENT_TAGS lists the tags necessary to build the trace-agent
TRACE_AGENT_TAGS = set([
    "docker",
    "containerd",
    "datadog.no_waf",
    "kubelet",
    "otlp",
    "netcgo",
    "podman",
])

# TRACE_AGENT_HEROKU_TAGS lists the tags necessary to build the trace-agent for Heroku
TRACE_AGENT_HEROKU_TAGS = TRACE_AGENT_TAGS.difference(
    set([
        "containerd",
        "docker",
        "kubeapiserver",
        "kubelet",
        "podman",
    ]),
)

CWS_INSTRUMENTATION_TAGS = set(["netgo", "osusergo"])

OTEL_AGENT_TAGS = set(["otlp", "zlib", "zstd", "kubelet"])

LOADER_TAGS = set()

# We need to remove all discovery provider from prometheusreceiver components to avoid loading too many dependencies in the host-profiler binary.
# imported by https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/f963ab53ee55aeb56d58617ed12c840e8b07cc53/receiver/prometheusreceiver/factory.go#L10
HOST_PROFILER_TAGS = set(["remove_all_sd", "docker", "kubelet"])

PRIVATEACTIONRUNNER_TAGS = set(["zlib", "zstd"])

SECRET_GENERIC_CONNECTOR_TAGS = set()

# AGENT_TEST_TAGS lists the tags that have to be added to run tests
AGENT_TEST_TAGS = AGENT_TAGS.union(set(["clusterchecks"]))

### Tag exclusion lists

# List of tags to always remove when not building on Linux
LINUX_ONLY_TAGS = set(["netcgo", "systemd", "jetson", "linux_bpf", "nvml", "pcap", "podman", "trivy", "crio"])

# List of tags to always remove when building on AIX
AIX_EXCLUDED_TAGS = set([
    "cel",
    "clusterchecks",
    "containerd",
    "cri",
    "crio",
    "docker",
    "fargateprocess",
    "jetson",
    "jmx",
    "kubeapiserver",
    "kubelet",
    "linux_bpf",
    "netcgo",
    "npm",
    "nvml",
    "orchestrator",
    "pcap",
    "podman",
    "sharedlibrarycheck",
    "systemd",
    "systemprobechecks",
    "trivy",
])

# List of tags to always add when building on Windows
WINDOWS_INCLUDED_TAGS = set(["wmi"])

# List of tags to always remove when building on Windows
WINDOWS_EXCLUDED_TAGS = set([
    "requirefips",
])

# List of tags to always remove when building on Darwin/macOS
DARWIN_EXCLUDED_TAGS = set(["docker", "containerd", "cri"])

# Unit test build tags
UNIT_TEST_TAGS = set(["test"])

# List of tags to always remove when running unit tests
UNIT_TEST_EXCLUDED_TAGS = set(["datadog.no_waf", "pcap"])

### Per-flavor unit-test tag sets

def _unit_test_tags(flavor_tags):
    return sorted(((flavor_tags | UNIT_TEST_TAGS) - UNIT_TEST_EXCLUDED_TAGS) | COMMON_TAGS)

# FLAVOR_UNIT_TEST_TAGS maps each AgentFlavor name to the build tags used when
# running its unit tests. It mirrors the build_tags[flavor]["unit-tests"] entries
# in tasks/build_tags.py: the flavor's build set unioned with UNIT_TEST_TAGS,
# minus UNIT_TEST_EXCLUDED_TAGS, then unioned with COMMON_TAGS (as
# get_default_build_tags() does). LINUX_ONLY tags are kept here; per-platform
# filtering is applied by flavor_gotags() in //bazel/flavors:defs.bzl. Consumed
# by that macro (Starlark load) and, via tasks/build_tags.py, by the
# dd_agent_go_test Gazelle extension's generated tags.go. Kept in sync with
# build_tags.py by tasks/unit_tests/build_tags_tests.py.
FLAVOR_UNIT_TEST_TAGS = {
    "base": _unit_test_tags(AGENT_TEST_TAGS | PROCESS_AGENT_TAGS | CLUSTER_AGENT_TAGS),
    "fips": _unit_test_tags(AGENT_TAGS | FIPS_TAGS),
    "heroku": _unit_test_tags(AGENT_HEROKU_TAGS),
    "iot": _unit_test_tags(IOT_AGENT_TAGS),
    "dogstatsd": _unit_test_tags(DOGSTATSD_TAGS),
}
