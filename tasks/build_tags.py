"""
Utilities to manage build tags
"""
# TODO: check if we really need the typing import.
# Recent versions of Python should be able to use dict and list directly in type hints,
# so we only need to check that we don't run this code with old Python versions.

import sys
from typing import Union

from invoke import task
from invoke.exceptions import Exit

from .flavor import AgentFlavor

# ALL_TAGS lists all available build tags.
# Used to remove unknown tags from provided tag lists.
ALL_TAGS = {
    "apm",
    "clusterchecks",
    "consul",
    "containerd",
    "cri",
    "docker",
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

# CLUSTER_AGENT_TAGS lists the tags needed when building the cluster-agent
CLUSTER_AGENT_TAGS = {"clusterchecks", "kubeapiserver", "orchestrator", "zlib", "ec2", "gce"}

# CLUSTER_AGENT_CLOUDFOUNDRY_TAGS lists the tags needed when building the cloudfoundry cluster-agent
CLUSTER_AGENT_CLOUDFOUNDRY_TAGS = {"clusterchecks"}

# DOGSTATSD_TAGS lists the tags needed when building dogstatsd
DOGSTATSD_TAGS = {"containerd", "docker", "kubelet", "podman", "zlib"}

# IOT_AGENT_TAGS lists the tags needed when building the IoT agent
IOT_AGENT_TAGS = {"jetson", "otlp", "systemd", "zlib"}

# PROCESS_AGENT_TAGS lists the tags necessary to build the process-agent
PROCESS_AGENT_TAGS = AGENT_TAGS.union({"clusterchecks", "fargateprocess", "orchestrator"}).difference(
    {"otlp", "python", "trivy"}
)

# PROCESS_AGENT_HEROKU_TAGS lists the tags necessary to build the process-agent for Heroku
PROCESS_AGENT_HEROKU_TAGS = PROCESS_AGENT_TAGS.difference(
    {"containerd", "cri", "docker", "ec2", "jetson", "kubeapiserver", "kubelet", "orchestrator", "podman", "systemd"}
)

# SECURITY_AGENT_TAGS lists the tags necessary to build the security agent
SECURITY_AGENT_TAGS = {"netcgo", "docker", "containerd", "kubeapiserver", "kubelet", "podman", "zlib", "ec2"}

# SERVERLESS_TAGS lists the tags necessary to build serverless
SERVERLESS_TAGS = {"serverless", "otlp"}

# SYSTEM_PROBE_TAGS lists the tags necessary to build system-probe
SYSTEM_PROBE_TAGS = AGENT_TAGS.union({"clusterchecks", "linux_bpf", "npm"}).difference({"python", "systemd"})

# TRACE_AGENT_TAGS lists the tags that have to be added when the trace-agent
TRACE_AGENT_TAGS = {"docker", "containerd", "kubeapiserver", "kubelet", "otlp", "netcgo", "podman"}

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
        "system-probe-unit-tests": SYSTEM_PROBE_TAGS.union(UNIT_TEST_TAGS),
        "trace-agent": TRACE_AGENT_TAGS,
        # Test setups
        "test": AGENT_TEST_TAGS.union(UNIT_TEST_TAGS),
        "lint": AGENT_TEST_TAGS.union(PROCESS_AGENT_TAGS).union(UNIT_TEST_TAGS),
        "unit-tests": AGENT_TEST_TAGS.union(PROCESS_AGENT_TAGS).union(UNIT_TEST_TAGS),
    },
    AgentFlavor.heroku: {
        "agent": AGENT_HEROKU_TAGS,
        "process-agent": PROCESS_AGENT_HEROKU_TAGS,
        "trace-agent": TRACE_AGENT_HEROKU_TAGS,
        "lint": AGENT_HEROKU_TAGS.union(UNIT_TEST_TAGS),
        "unit-tests": AGENT_HEROKU_TAGS.union(UNIT_TEST_TAGS),
    },
    AgentFlavor.iot: {
        "agent": IOT_AGENT_TAGS,
        "lint": IOT_AGENT_TAGS.union(UNIT_TEST_TAGS),
        "unit-tests": IOT_AGENT_TAGS.union(UNIT_TEST_TAGS),
    },
    AgentFlavor.dogstatsd: {
        "dogstatsd": DOGSTATSD_TAGS,
        "system-tests": AGENT_TAGS,
        "lint": DOGSTATSD_TAGS.union(UNIT_TEST_TAGS),
        "unit-tests": DOGSTATSD_TAGS.union(UNIT_TEST_TAGS),
    },
}


def get_build_tags(
    build: str = "agent",
    arch: str = "x64",
    flavor: AgentFlavor = AgentFlavor.base,
    build_include: str = None,
    build_exclude: str = None,
    platform: str = sys.platform,
) -> list[str]:
    """
    Compute the list of go build tags to use, based on the build type, flavor,
    and explicit include / exclude tags passed to the function.
    """

    # Check that the flavor x build combination is supported
    if build not in build_tags[flavor]:
        print(f"{build} is not a valid target for flavor {flavor.name}. Valid targets are: \n")
        print(f'{", ".join(build_tags[flavor].keys())} \n')
        raise Exit(code=1)

    build_include = (
        _default_build_tags(build=build, arch=arch, flavor=flavor, platform=platform)
        if build_include is None
        else _filter_incompatible_tags(build_include.split(","), arch=arch, platform=platform)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")
    return _compute_build_tags(build_include, build_exclude)


@task
def print_build_tags(
    _,
    build: str = "agent",
    arch: str = "x64",
    flavor: str = AgentFlavor.base.name,
    build_include: str = None,
    build_exclude: str = None,
    platform: str = sys.platform,
):
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
        print(f"'{flavor}' does not correspond to an agent flavor. Options:\n")
        print(f'{", ".join(flavorOptions)} \n')
        raise Exit(code=1)

    print(
        ",".join(
            sorted(
                get_build_tags(
                    build, arch, flavor, platform=platform, build_include=build_include, build_exclude=build_exclude
                )
            )
        )
    )


def _default_build_tags(
    build: str = "agent", arch: str = "x64", flavor: AgentFlavor = AgentFlavor.base, platform: str = sys.platform
) -> list[str]:
    """
    Build the default list of tags based on the build type and current platform.

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """
    include = build_tags.get(flavor).get(build)

    return _filter_incompatible_tags(include, arch=arch, platform=platform)


def _filter_incompatible_tags(
    include: Union[list[str], set[str]], arch: str = "x64", platform: str = sys.platform
) -> list[str]:
    """
    Filter out tags incompatible with the platform.
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

    return _compute_build_tags(include, exclude)


def _compute_build_tags(include: Union[list[str], set[str]], exclude: Union[list[str], set[str]]) -> list[str]:
    """
    Build the sorted list of tags based on inclusions and exclusions passed through
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

    return sorted(known_include - known_exclude)


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
