"""
Utilities to manage build tags
"""
# TODO: check if we really need the typing import.
# Recent versions of Python should be able to use dict and list directly in type hints,
# so we only need to check that we don't run this code with old Python versions.

import sys
from typing import List

from invoke import task

from tasks.flavor import AgentFlavor

# ALL_TAGS lists all available build tags.
# Used to remove unknown tags from provided tag lists.
ALL_TAGS = {
    "apm",
    "clusterchecks",
    "consul",
    "containerd",
    "cri",
    "docker",
    "datadog.no_waf",
    "ec2",
    "etcd",
    "fargateprocess",
    "gce",
    "jmx",
    "jetson",
    "kubeapiserver",
    "kubelet",
    "linux_bpf",
    "netcgo",  # Force the use of the CGO resolver. This will also have the effect of making the binary non-static
    "npm",
    "oracle",
    "orchestrator",
    "otlp",
    "podman",
    "process",
    "python",
    "serverless",
    "systemd",
    "trivy",
    "zk",
    "zlib",
    "test",  # used for unit-tests
}

### Tag inclusion lists

# AGENT_TAGS lists the tags needed when building the agent.
AGENT_TAGS = {
    "apm",
    "consul",
    "containerd",
    "cri",
    "datadog.no_waf",
    "docker",
    "ec2",
    "etcd",
    "gce",
    "jetson",
    "jmx",
    "kubeapiserver",
    "kubelet",
    "netcgo",
    "oracle",
    "orchestrator",
    "otlp",
    "podman",
    "process",
    "python",
    "systemd",
    "trivy",
    "zk",
    "zlib",
}

# AGENT_HEROKU_TAGS lists the tags for Heroku agent build
AGENT_HEROKU_TAGS = AGENT_TAGS.difference(
    {
        "containerd",
        "cri",
        "docker",
        "ec2",
        "jetson",
        "kubeapiserver",
        "kubelet",
        "oracle",
        "orchestrator",
        "podman",
        "systemd",
        "trivy",
    }
)

# AGENTLESS_SCANNER_TAGS lists the tags needed when building the agentless-scanner
AGENTLESS_SCANNER_TAGS = {""}

# CLUSTER_AGENT_TAGS lists the tags needed when building the cluster-agent
CLUSTER_AGENT_TAGS = {"clusterchecks", "datadog.no_waf", "kubeapiserver", "orchestrator", "zlib", "ec2", "gce"}

# CLUSTER_AGENT_CLOUDFOUNDRY_TAGS lists the tags needed when building the cloudfoundry cluster-agent
CLUSTER_AGENT_CLOUDFOUNDRY_TAGS = {"clusterchecks"}

# DOGSTATSD_TAGS lists the tags needed when building dogstatsd
DOGSTATSD_TAGS = {"containerd", "docker", "kubelet", "podman", "zlib"}

# IOT_AGENT_TAGS lists the tags needed when building the IoT agent
IOT_AGENT_TAGS = {"jetson", "otlp", "systemd", "zlib"}

# PROCESS_AGENT_TAGS lists the tags necessary to build the process-agent
PROCESS_AGENT_TAGS = AGENT_TAGS.union({"fargateprocess"}).difference({"otlp", "python", "trivy"})

# PROCESS_AGENT_HEROKU_TAGS lists the tags necessary to build the process-agent for Heroku
PROCESS_AGENT_HEROKU_TAGS = PROCESS_AGENT_TAGS.difference(
    {"containerd", "cri", "docker", "ec2", "jetson", "kubeapiserver", "kubelet", "orchestrator", "podman", "systemd"}
)

# SECURITY_AGENT_TAGS lists the tags necessary to build the security agent
SECURITY_AGENT_TAGS = {
    "netcgo",
    "datadog.no_waf",
    "docker",
    "containerd",
    "kubeapiserver",
    "kubelet",
    "podman",
    "zlib",
    "ec2",
}

# SERVERLESS_TAGS lists the tags necessary to build serverless
SERVERLESS_TAGS = {"serverless", "otlp"}

# SYSTEM_PROBE_TAGS lists the tags necessary to build system-probe
SYSTEM_PROBE_TAGS = AGENT_TAGS.union({"linux_bpf", "npm"}).difference({"python", "systemd"})

# TRACE_AGENT_TAGS lists the tags that have to be added when the trace-agent
TRACE_AGENT_TAGS = {"docker", "containerd", "datadog.no_waf", "kubeapiserver", "kubelet", "otlp", "netcgo", "podman"}

# TRACE_AGENT_HEROKU_TAGS lists the tags necessary to build the trace-agent for Heroku
TRACE_AGENT_HEROKU_TAGS = TRACE_AGENT_TAGS.difference(
    {
        "containerd",
        "docker",
        "kubeapiserver",
        "kubelet",
        "podman",
    }
)

# AGENT_TEST_TAGS lists the tags that have to be added to run tests
AGENT_TEST_TAGS = AGENT_TAGS.union({"clusterchecks"})


### Tag exclusion lists

# List of tags to always remove when not building on Linux
LINUX_ONLY_TAGS = {"netcgo", "systemd", "jetson", "linux_bpf", "podman", "trivy"}

# List of tags to always remove when building on Windows
WINDOWS_EXCLUDE_TAGS = {"linux_bpf"}

# List of tags to always remove when building on Darwin/macOS
DARWIN_EXCLUDED_TAGS = {"docker", "containerd", "cri"}

# List of tags to always remove when building on Windows 32-bits
WINDOWS_32BIT_EXCLUDE_TAGS = {"docker", "kubeapiserver", "kubelet", "orchestrator"}

# Unit test build tags
UNIT_TEST_TAGS = {"test"}

# List of tags to always remove when running unit tests
UNIT_TEST_EXCLUDE_TAGS = {"datadog.no_waf"}

# Build type: maps flavor to build tags map
build_tags = {
    AgentFlavor.base: {
        # Build setups
        "agent": AGENT_TAGS,
        "cluster-agent": CLUSTER_AGENT_TAGS,
        "cluster-agent-cloudfoundry": CLUSTER_AGENT_CLOUDFOUNDRY_TAGS,
        "dogstatsd": DOGSTATSD_TAGS,
        "process-agent": PROCESS_AGENT_TAGS,
        "security-agent": SECURITY_AGENT_TAGS,
        "serverless": SERVERLESS_TAGS,
        "system-probe": SYSTEM_PROBE_TAGS,
        "system-probe-unit-tests": SYSTEM_PROBE_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "trace-agent": TRACE_AGENT_TAGS,
        # Test setups
        "test": AGENT_TEST_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "lint": AGENT_TEST_TAGS.union(PROCESS_AGENT_TAGS).union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "unit-tests": AGENT_TEST_TAGS.union(PROCESS_AGENT_TAGS)
        .union(UNIT_TEST_TAGS)
        .difference(UNIT_TEST_EXCLUDE_TAGS),
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
    AgentFlavor.agentless_scanner: {
        "dogstatsd": AGENTLESS_SCANNER_TAGS,
        "system-tests": AGENT_TAGS,
        "lint": AGENTLESS_SCANNER_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
        "unit-tests": AGENTLESS_SCANNER_TAGS.union(UNIT_TEST_TAGS).difference(UNIT_TEST_EXCLUDE_TAGS),
    },
}


def compute_build_tags_for_flavor(
    build: str,
    arch: str,
    build_include: List[str],
    build_exclude: List[str],
    flavor: AgentFlavor = AgentFlavor.base,
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
        get_default_build_tags(build=build, arch=arch, flavor=flavor)
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), arch=arch)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    return get_build_tags(build_include, build_exclude)


@task
def print_default_build_tags(_, build="agent", arch="x64", flavor=AgentFlavor.base.name):
    """
    Build the default list of tags based on the build type and current platform.
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

    print(",".join(sorted(get_default_build_tags(build, arch, flavor))))


def get_default_build_tags(build="agent", arch="x64", flavor=AgentFlavor.base, platform=sys.platform):
    """
    Build the default list of tags based on the build type and current platform.

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """
    include = build_tags.get(flavor).get(build)
    if include is None:
        print("Warning: unrecognized build type, no build tags included.", file=sys.stderr)
        include = set()

    return sorted(filter_incompatible_tags(include, arch=arch, platform=platform))


def filter_incompatible_tags(include, arch="x64", platform=sys.platform):
    """
    Filter out tags incompatible with the platform.
    include can be a list or a set.
    """

    exclude = set()
    if not platform.startswith("linux"):
        exclude = exclude.union(LINUX_ONLY_TAGS)

    if platform == "win32":
        exclude = exclude.union(WINDOWS_EXCLUDE_TAGS)

    if platform == "darwin":
        exclude = exclude.union(DARWIN_EXCLUDED_TAGS)

    if platform == "win32" and arch == "x86":
        exclude = exclude.union(WINDOWS_32BIT_EXCLUDE_TAGS)

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
