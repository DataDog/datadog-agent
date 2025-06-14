"""
Utilities to manage build tags
"""

# TODO: check if we really need the typing import.
# Recent versions of Python should be able to use dict and list directly in type hints,
# so we only need to check that we don't run this code with old Python versions.
from __future__ import annotations

import os
import sys

from invoke import task

from tasks.flavor import AgentFlavor

# ALL_TAGS lists all available build tags.
# Used to remove unknown tags from provided tag lists.
ALL_TAGS = {
    "bundle_installer",
    "clusterchecks",
    "consul",
    "containerd",
    # Disables dynamic plugins in containerd v1, which removes the import to std "plugin" package on Linux amd64,
    # which makes the agent significantly smaller.
    # This can be removed when we start using containerd v2.1 or later.
    "no_dynamic_plugins",
    "cri",
    "crio",
    "datadog.no_waf",
    "docker",
    "ec2",
    "etcd",
    "fargateprocess",
    "goexperiment.systemcrypto",  # used for FIPS mode
    # removes the import to golang.org/x/net/trace in google.golang.org/grpc,
    # which prevents dead code elimination, see https://github.com/golang/go/issues/62024
    "grpcnotrace",
    "jetson",
    "jmx",
    "kubeapiserver",
    "kubelet",
    "linux_bpf",
    "netcgo",  # Force the use of the CGO resolver. This will also have the effect of making the binary non-static
    "netgo",
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
    "sds",
    "serverless",
    "serverlessfips",  # used for FIPS mode in the serverless build in datadog-lambda-extension
    "systemd",
    "test",  # used for unit-tests
    "trivy",
    "trivy_no_javadb",
    "wmi",
    "zk",
    "zlib",
    "zstd",
}

### Tag inclusion lists

# AGENT_TAGS lists the tags needed when building the agent.
AGENT_TAGS = {
    "bundle_installer",
    "consul",
    "containerd",
    "no_dynamic_plugins",
    "cri",
    "crio",
    "datadog.no_waf",
    "docker",
    "ec2",
    "etcd",
    "fargateprocess",
    "grpcnotrace",
    "jetson",
    "jmx",
    "kubeapiserver",
    "kubelet",
    "netcgo",
    "nvml",
    "oracle",
    "orchestrator",
    "otlp",
    "podman",
    "python",
    "systemd",
    "trivy",
    "trivy_no_javadb",
    "zk",
    "zlib",
    "zstd",
}

# AGENT_HEROKU_TAGS lists the tags for Heroku agent build
AGENT_HEROKU_TAGS = AGENT_TAGS.difference(
    {
        "containerd",
        "no_dynamic_plugins",
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
    }
)

FIPS_TAGS = {"goexperiment.systemcrypto", "requirefips"}

# CLUSTER_AGENT_TAGS lists the tags needed when building the cluster-agent
CLUSTER_AGENT_TAGS = {
    "clusterchecks",
    "datadog.no_waf",
    "grpcnotrace",
    "kubeapiserver",
    "orchestrator",
    "zlib",
    "zstd",
    "ec2",
}

# CLUSTER_AGENT_CLOUDFOUNDRY_TAGS lists the tags needed when building the cloudfoundry cluster-agent
CLUSTER_AGENT_CLOUDFOUNDRY_TAGS = {"clusterchecks", "grpcnotrace"}

# DOGSTATSD_TAGS lists the tags needed when building dogstatsd
DOGSTATSD_TAGS = {"containerd", "grpcnotrace", "no_dynamic_plugins", "docker", "kubelet", "podman", "zlib", "zstd"}

# IOT_AGENT_TAGS lists the tags needed when building the IoT agent
IOT_AGENT_TAGS = {"grpcnotrace", "jetson", "otlp", "systemd", "zlib", "zstd"}

# INSTALLER_TAGS lists the tags needed when building the installer
INSTALLER_TAGS = {"docker", "ec2", "kubelet"}

# PROCESS_AGENT_TAGS lists the tags necessary to build the process-agent
PROCESS_AGENT_TAGS = {
    "containerd",
    "no_dynamic_plugins",
    "cri",
    "crio",
    "datadog.no_waf",
    "ec2",
    "docker",
    "fargateprocess",
    "grpcnotrace",
    "kubelet",
    "netcgo",
    "podman",
    "zlib",
    "zstd",
}

# PROCESS_AGENT_HEROKU_TAGS lists the tags necessary to build the process-agent for Heroku
PROCESS_AGENT_HEROKU_TAGS = {
    "datadog.no_waf",
    "fargateprocess",
    "grpcnotrace",
    "netcgo",
    "zlib",
    "zstd",
}

# SECURITY_AGENT_TAGS lists the tags necessary to build the security agent
SECURITY_AGENT_TAGS = {
    "netcgo",
    "datadog.no_waf",
    "docker",
    "grpcnotrace",
    "zlib",
    "zstd",
    "ec2",
}

# SBOMGEN_TAGS lists the tags necessary to build sbomgen
SBOMGEN_TAGS = {
    "trivy",
    "trivy_no_javadb",
    "grpcnotrace",
    "no_dynamic_plugins",
    "containerd",
    "docker",
    "crio",
}

# SERVERLESS_TAGS lists the tags necessary to build serverless
SERVERLESS_TAGS = {"grpcnotrace", "serverless", "otlp"}

# SYSTEM_PROBE_TAGS lists the tags necessary to build system-probe
SYSTEM_PROBE_TAGS = {
    "datadog.no_waf",
    "ec2",
    "grpcnotrace",
    "linux_bpf",
    "netcgo",
    "npm",
    "nvml",
    "pcap",
    "trivy",
    "trivy_no_javadb",
    "zlib",
    "zstd",
}

# TRACE_AGENT_TAGS lists the tags that have to be added when the trace-agent
TRACE_AGENT_TAGS = {
    "docker",
    "containerd",
    "grpcnotrace",
    "no_dynamic_plugins",
    "datadog.no_waf",
    "kubeapiserver",
    "kubelet",
    "otlp",
    "netcgo",
    "podman",
}

# TRACE_AGENT_HEROKU_TAGS lists the tags necessary to build the trace-agent for Heroku
TRACE_AGENT_HEROKU_TAGS = TRACE_AGENT_TAGS.difference(
    {
        "containerd",
        "no_dynamic_plugins",
        "docker",
        "kubeapiserver",
        "kubelet",
        "podman",
    }
)

CWS_INSTRUMENTATION_TAGS = {"netgo", "osusergo"}

OTEL_AGENT_TAGS = {"otlp", "zlib", "zstd"}

# AGENT_TEST_TAGS lists the tags that have to be added to run tests
AGENT_TEST_TAGS = AGENT_TAGS.union({"clusterchecks"})


### Tag exclusion lists

# List of tags to always remove when not building on Linux
LINUX_ONLY_TAGS = {"netcgo", "systemd", "jetson", "linux_bpf", "nvml", "pcap", "podman", "trivy"}

# List of tags to always remove when building on Windows
WINDOWS_EXCLUDE_TAGS = {"linux_bpf", "nvml", "requirefips"}

# List of tags to always remove when building on Darwin/macOS
DARWIN_EXCLUDED_TAGS = {"docker", "containerd", "no_dynamic_plugins", "nvml", "cri", "crio"}

# Unit test build tags
UNIT_TEST_TAGS = {"test"}

# List of tags to always remove when running unit tests
UNIT_TEST_EXCLUDE_TAGS = {"datadog.no_waf", "pcap"}

# Build type: maps flavor to build tags map
build_tags = {
    AgentFlavor.base: {
        # Build setups
        "agent": AGENT_TAGS,
        "cluster-agent": CLUSTER_AGENT_TAGS,
        "cluster-agent-cloudfoundry": CLUSTER_AGENT_CLOUDFOUNDRY_TAGS,
        "dogstatsd": DOGSTATSD_TAGS,
        "installer": INSTALLER_TAGS,
        "process-agent": PROCESS_AGENT_TAGS,
        "security-agent": SECURITY_AGENT_TAGS,
        "serverless": SERVERLESS_TAGS,
        "system-probe": SYSTEM_PROBE_TAGS,
        "system-probe-unit-tests": SYSTEM_PROBE_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "trace-agent": TRACE_AGENT_TAGS,
        "cws-instrumentation": CWS_INSTRUMENTATION_TAGS,
        "sbomgen": SBOMGEN_TAGS,
        "otel-agent": OTEL_AGENT_TAGS,
        # Test setups
        "test": AGENT_TEST_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "lint": AGENT_TEST_TAGS.union(PROCESS_AGENT_TAGS).union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "unit-tests": AGENT_TEST_TAGS.union(PROCESS_AGENT_TAGS)
        .union(UNIT_TEST_TAGS)
        .difference(UNIT_TEST_EXCLUDE_TAGS),
    },
    AgentFlavor.fips: {
        "agent": AGENT_TAGS.union(FIPS_TAGS),
        "dogstatsd": DOGSTATSD_TAGS.union(FIPS_TAGS),
        "process-agent": PROCESS_AGENT_TAGS.union(FIPS_TAGS),
        "security-agent": SECURITY_AGENT_TAGS.union(FIPS_TAGS),
        "serverless": SERVERLESS_TAGS.union(FIPS_TAGS),
        "system-probe": SYSTEM_PROBE_TAGS.union(FIPS_TAGS),
        "system-probe-unit-tests": SYSTEM_PROBE_TAGS.union(FIPS_TAGS)
        .union(UNIT_TEST_TAGS)
        .difference(UNIT_TEST_EXCLUDE_TAGS),
        "trace-agent": TRACE_AGENT_TAGS.union(FIPS_TAGS),
        "cws-instrumentation": CWS_INSTRUMENTATION_TAGS.union(FIPS_TAGS),
        "sbomgen": SBOMGEN_TAGS.union(FIPS_TAGS),
        # Test setups
        "lint": AGENT_TAGS.union(FIPS_TAGS).union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "unit-tests": AGENT_TAGS.union(FIPS_TAGS).union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
    },
    AgentFlavor.heroku: {
        "agent": AGENT_HEROKU_TAGS,
        "process-agent": PROCESS_AGENT_HEROKU_TAGS,
        "trace-agent": TRACE_AGENT_HEROKU_TAGS,
        "lint": AGENT_HEROKU_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "unit-tests": AGENT_HEROKU_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
    },
    AgentFlavor.iot: {
        "agent": IOT_AGENT_TAGS,
        "lint": IOT_AGENT_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "unit-tests": IOT_AGENT_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
    },
    AgentFlavor.dogstatsd: {
        "dogstatsd": DOGSTATSD_TAGS,
        "system-tests": AGENT_TAGS,
        "lint": DOGSTATSD_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "unit-tests": DOGSTATSD_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
    },
}


def compute_build_tags_for_flavor(
    build: str,
    build_include: str | None,
    build_exclude: str | None,
    flavor: AgentFlavor = AgentFlavor.base,
    include_sds: bool = False,
):
    """
    Given a flavor, an architecture, a list of tags to include and exclude, get the final list
    of tags that should be applied.
    If the list of build tags to include is empty, take the default list of build tags for
    the flavor or arch. Otherwise, use the list of build tags to include, minus incompatible tags
    for the given architecture.

    Then, remove from these the provided list of tags to exclude.
    """
    build_include = (
        get_default_build_tags(build=build, flavor=flavor)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","))
    )

    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    list = get_build_tags(build_include, build_exclude)

    if include_sds:
        list.append("sds")

    return list


@task
def print_default_build_tags(_, build="agent", flavor=AgentFlavor.base.name, platform: str | None = None):
    """
    Build the default list of tags based on the build type and platform.
    Prints as comma separated list suitable for go tooling (eg, gopls, govulncheck)

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """

    try:
        flavor = AgentFlavor[flavor]
    except KeyError:
        flavorOptions = [flavor.name for flavor in AgentFlavor]
        print(f"'{flavor}' does not correspond to an agent flavor. Options: {flavorOptions}")
        exit(1)

    print(",".join(sorted(get_default_build_tags(build=build, flavor=flavor, platform=platform))))


def get_default_build_tags(build="agent", flavor=AgentFlavor.base, platform: str | None = None):
    """
    Build the default list of tags based on the build type and current platform.

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """
    platform = platform or sys.platform
    include = build_tags[flavor].get(build)
    if include is None:
        print("Warning: unrecognized build type, no build tags included.", file=sys.stderr)
        include = set()

    return sorted(filter_incompatible_tags(include, platform=platform))


def filter_incompatible_tags(include, platform=sys.platform):
    """
    Filter out tags incompatible with the platform.
    include can be a list or a set.
    """

    exclude = set()
    if not platform.startswith("linux"):
        exclude = exclude.union(LINUX_ONLY_TAGS)

    if platform == "win32" or os.getenv("GOOS") == "windows":
        include = include.union(["wmi"])
        exclude = exclude.union(WINDOWS_EXCLUDE_TAGS)

    if platform == "darwin":
        exclude = exclude.union(DARWIN_EXCLUDED_TAGS)

    return get_build_tags(include, exclude)


def get_build_tags(include, exclude):
    """
    Build the list of tags based on inclusions and exclusions passed through
    the command line
    include and exclude can be lists or sets.
    """
    # Convert parameters to sets
    include = set(include)
    exclude = set(exclude)

    # filter out unrecognised tags
    known_include = ALL_TAGS.intersection(include)
    unknown_include = include - known_include
    for tag in unknown_include:
        print(f"Warning: unknown build tag '{tag}' was filtered out from included tags list.", file=sys.stderr)

    known_exclude = ALL_TAGS.intersection(exclude)
    unknown_exclude = exclude - known_exclude
    for tag in unknown_exclude:
        print(f"Warning: unknown build tag '{tag}' was filtered out from excluded tags list.", file=sys.stderr)

    return list(known_include - known_exclude)


@task
def audit_tag_impact(ctx, build_exclude=None, csv=False):
    """
    Measure each tag's contribution to the binary size
    """
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    tags_to_audit = ALL_TAGS.difference(set(build_exclude)).difference(set(IOT_AGENT_TAGS))

    max_size = _compute_build_size(ctx, build_exclude=','.join(build_exclude))
    print(f"size with all tags is {max_size / 1000} kB")

    iot_agent_size = _compute_build_size(ctx, flavor=AgentFlavor.iot)
    print(f"iot agent size is {iot_agent_size / 1000} kB\n")

    report = {"unaccounted": max_size - iot_agent_size, "iot_agent": iot_agent_size}

    for tag in tags_to_audit:
        exclude_string = ','.join(build_exclude + [tag])
        size = _compute_build_size(ctx, build_exclude=exclude_string)
        delta = max_size - size
        print(f"tag {tag} adds {delta / 1000} kB (excludes: {exclude_string})")
        report[tag] = delta
        report["unaccounted"] -= delta

    if csv:
        print("\nCSV output in bytes:")
        for k, v in report.items():
            print(f"{k};{v}")


def _compute_build_size(ctx, build_exclude=None, flavor=AgentFlavor.base):
    import os

    from .agent import build as agent_build

    agent_build(ctx, build_exclude=build_exclude, skip_assets=True, flavor=flavor)

    statinfo = os.stat('bin/agent/agent')
    return statinfo.st_size


def compute_config_build_tags(targets="all", build_include=None, build_exclude=None, flavor=AgentFlavor.base.name):
    flavor = AgentFlavor[flavor]

    if targets == "all":
        targets = build_tags[flavor].keys()
    else:
        targets = targets.split(",")
        if not set(targets).issubset(build_tags[flavor]):
            print("Must choose valid targets. Valid targets are:")
            print(f'{", ".join(build_tags[flavor].keys())}')
            exit(1)

    if build_include is None:
        build_include = []
        for target in targets:
            build_include.extend(get_default_build_tags(build=target, flavor=flavor))
    else:
        build_include = filter_incompatible_tags(build_include.split(","))

    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    use_tags = get_build_tags(build_include, build_exclude)
    return use_tags


def add_fips_tags(tags: list[str], fips_mode: bool) -> list[str]:
    is_windows_build = sys.platform == 'win32' or os.getenv("GOOS") == "windows"
    if fips_mode:
        tags.append("goexperiment.systemcrypto")
    if fips_mode and not is_windows_build:
        tags.append("requirefips")
    return tags
