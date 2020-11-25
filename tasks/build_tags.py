"""
Utilities to manage build tags
"""
import sys

from invoke import task

# ALL_TAGS lists all available build tags.
# Used to remove unknown tags from provided tag lists.
ALL_TAGS = set(
    [
        "android",
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
        "orchestrator",
        "process",
        "python",
        "secrets",
        "systemd",
        "zk",
        "zlib",
    ]
)

### Tag inclusion lists

# AGENT_TAGS lists the tags needed when building the agent.
AGENT_TAGS = set(
    [
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
        "process",
        "python",
        "secrets",
        "systemd",
        "zk",
        "zlib",
    ]
)

# ANDROID_TAGS lists the tags needed when building the android agent
ANDROID_TAGS = set(["android", "zlib",])

# CLUSTER_AGENT_TAGS lists the tags needed when building the cluster-agent
CLUSTER_AGENT_TAGS = set(["clusterchecks", "kubeapiserver", "orchestrator", "secrets", "zlib",])

# CLUSTER_AGENT_CLOUDFOUNDRY_TAGS lists the tags needed when building the cloudfoundry cluster-agent
CLUSTER_AGENT_CLOUDFOUNDRY_TAGS = set(["clusterchecks", "secrets",])

# DOGSTATSD_TAGS lists the tags needed when building dogstatsd
DOGSTATSD_TAGS = set(["docker", "kubelet", "secrets", "zlib",])

# IOT_AGENT_TAGS lists the tags needed when building the IoT agent
IOT_AGENT_TAGS = set(["jetson", "systemd", "zlib",])

# PROCESS_AGENT_TAGS lists the tags necessary to build the process-agent
PROCESS_AGENT_TAGS = AGENT_TAGS.union(set(["clusterchecks", "fargateprocess", "orchestrator",]))

# SECURITY_AGENT_TAGS lists the tags necessary to build the security agent
SECURITY_AGENT_TAGS = set(["netcgo", "secrets", "docker", "kubeapiserver", "kubelet",])

# PROCESS_AGENT_TAGS lists the tags necessary to build system-probe
SYSTEM_PROBE_TAGS = AGENT_TAGS.union(set(["clusterchecks", "linux_bpf",]))

# TRACE_AGENT_TAGS lists the tags that have to be added when the trace-agent
TRACE_AGENT_TAGS = set(["docker", "kubeapiserver", "kubelet", "netcgo", "secrets",])

# TEST_TAGS lists the tags that have to be added to run tests
TEST_TAGS = AGENT_TAGS.union(set(["clusterchecks",]))

### Tag exclusion lists

# List of tags to always remove when not building on Linux
LINUX_ONLY_TAGS = set(["containerd", "cri", "netcgo", "systemd", "jetson",])

# List of tags to always remove when building on Windows
WINDOWS_EXCLUDE_TAGS = set(["linux_bpf"])

# List of tags to always remove when building on Windows 32-bits
WINDOWS_32BIT_EXCLUDE_TAGS = set(["docker", "kubeapiserver", "kubelet", "orchestrator",])

# Build type: build tags map
build_tags = {
    # Build setups
    "agent": AGENT_TAGS,
    "android": ANDROID_TAGS,
    "cluster-agent": CLUSTER_AGENT_TAGS,
    "cluster-agent-cloudfoundry": CLUSTER_AGENT_CLOUDFOUNDRY_TAGS,
    "dogstatsd": DOGSTATSD_TAGS,
    "iot": IOT_AGENT_TAGS,
    "process-agent": PROCESS_AGENT_TAGS,
    "security-agent": SECURITY_AGENT_TAGS,
    "system-probe": SYSTEM_PROBE_TAGS,
    "trace-agent": TRACE_AGENT_TAGS,
    # Test setups
    "test": TEST_TAGS,
    "test-with-process-tags": TEST_TAGS.union(PROCESS_AGENT_TAGS),
}


def get_default_build_tags(build="agent", arch="x64"):
    """
    Build the default list of tags based on the build type and current platform.

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """
    include = build_tags.get(build)
    if include is None:
        print("Warning: unrecognized build type, no build tags included.")
        include = set()

    return filter_incompatible_tags(include, arch=arch)


def filter_incompatible_tags(include, arch="x64"):
    """
    Filter out tags incompatible with the platform.
    include can be a list or a set.
    """

    exclude = set()
    if not sys.platform.startswith("linux"):
        exclude = exclude.union(LINUX_ONLY_TAGS)

    if sys.platform == "win32":
        exclude = exclude.union(WINDOWS_EXCLUDE_TAGS)

    if sys.platform == "win32" and arch == "x86":
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
        print("Warning: unknown build tag '{}' was filtered out from included tags list.".format(tag))

    known_exclude = ALL_TAGS.intersection(exclude)
    unknown_exclude = exclude - known_exclude
    for tag in unknown_exclude:
        print("Warning: unknown build tag '{}' was filtered out from excluded tags list.".format(tag))

    return list(known_include - known_exclude)


@task
def audit_tag_impact(ctx, build_exclude=None, csv=False):
    """
    Measure each tag's contribution to the binary size
    """
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    tags_to_audit = ALL_TAGS.difference(set(build_exclude)).difference(set(IOT_AGENT_TAGS))

    max_size = _compute_build_size(ctx, build_exclude=','.join(build_exclude))
    print("size with all tags is {} kB".format(max_size / 1000))

    iot_agent_size = _compute_build_size(ctx, iot=True)
    print("iot agent size is {} kB\n".format(iot_agent_size / 1000))

    report = {"unaccounted": max_size - iot_agent_size, "iot_agent": iot_agent_size}

    for tag in tags_to_audit:
        exclude_string = ','.join(build_exclude + [tag])
        size = _compute_build_size(ctx, build_exclude=exclude_string)
        delta = max_size - size
        print("tag {} adds {} kB (excludes: {})".format(tag, delta / 1000, exclude_string))
        report[tag] = delta
        report["unaccounted"] -= delta

    if csv:
        print("\nCSV output in bytes:")
        for k, v in report.items():
            print("{};{}".format(k, v))


def _compute_build_size(ctx, build_exclude=None, iot=False):
    import os

    from .agent import build as agent_build

    agent_build(ctx, build_exclude=build_exclude, skip_assets=True, iot=iot)

    statinfo = os.stat('bin/agent/agent')
    return statinfo.st_size
