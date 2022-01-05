"""
Utilities to manage build tags
"""
import sys

from invoke import task

from .flavor import AgentFlavor

# ALL_TAGS lists all available build tags.
# Used to remove unknown tags from provided tag lists.
ALL_TAGS = {
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
    "npm",
    "orchestrator",
    "otlp",
    "podman",
    "process",
    "python",
    "secrets",
    "systemd",
    "zk",
    "zlib",
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
    "orchestrator",
    "otlp",
    "podman",
    "process",
    "python",
    "secrets",
    "systemd",
    "zk",
    "zlib",
}

# AGENT_HEROKU_TAGS lists the tags for Heroku agent build
AGENT_HEROKU_TAGS = AGENT_TAGS.difference(
    {"containerd", "cri", "docker", "ec2", "jetson", "kubeapiserver", "kubelet", "podman", "systemd"}
)

# ANDROID_TAGS lists the tags needed when building the android agent
ANDROID_TAGS = {"android", "zlib"}

# CLUSTER_AGENT_TAGS lists the tags needed when building the cluster-agent
CLUSTER_AGENT_TAGS = {"clusterchecks", "kubeapiserver", "orchestrator", "secrets", "zlib", "ec2", "gce"}

# CLUSTER_AGENT_CLOUDFOUNDRY_TAGS lists the tags needed when building the cloudfoundry cluster-agent
CLUSTER_AGENT_CLOUDFOUNDRY_TAGS = {"clusterchecks", "secrets"}

# DOGSTATSD_TAGS lists the tags needed when building dogstatsd
DOGSTATSD_TAGS = {"containerd", "docker", "kubelet", "podman", "secrets", "zlib"}

# IOT_AGENT_TAGS lists the tags needed when building the IoT agent
IOT_AGENT_TAGS = {"jetson", "otlp", "systemd", "zlib"}

# PROCESS_AGENT_TAGS lists the tags necessary to build the process-agent
PROCESS_AGENT_TAGS = AGENT_TAGS.union({"clusterchecks", "fargateprocess", "orchestrator"}).difference({"otlp"})

# PROCESS_AGENT_HEROKU_TAGS lists the tags necessary to build the process-agent for Heroku
PROCESS_AGENT_HEROKU_TAGS = PROCESS_AGENT_TAGS.difference(
    {"containerd", "cri", "docker", "ec2", "jetson", "kubeapiserver", "kubelet", "podman", "systemd"}
)

# SECURITY_AGENT_TAGS lists the tags necessary to build the security agent
SECURITY_AGENT_TAGS = {"netcgo", "secrets", "docker", "containerd", "kubeapiserver", "kubelet", "podman"}

# SYSTEM_PROBE_TAGS lists the tags necessary to build system-probe
SYSTEM_PROBE_TAGS = AGENT_TAGS.union({"clusterchecks", "linux_bpf", "npm"})

# TRACE_AGENT_TAGS lists the tags that have to be added when the trace-agent
TRACE_AGENT_TAGS = {"docker", "containerd", "kubeapiserver", "kubelet", "otlp", "netcgo", "podman", "secrets"}

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

# TEST_TAGS lists the tags that have to be added to run tests
TEST_TAGS = AGENT_TAGS.union({"clusterchecks"})

### Tag exclusion lists

# List of tags to always remove when not building on Linux
LINUX_ONLY_TAGS = {"netcgo", "systemd", "jetson", "linux_bpf", "podman"}

# List of tags to always remove when building on Windows
WINDOWS_EXCLUDE_TAGS = {"linux_bpf"}

# List of tags to always remove when building on Windows 32-bits
WINDOWS_32BIT_EXCLUDE_TAGS = {"docker", "kubeapiserver", "kubelet", "orchestrator"}

# Build type: maps flavor to build tags map
build_tags = {
    AgentFlavor.base: {
        # Build setups
        "agent": AGENT_TAGS,
        "android": ANDROID_TAGS,
        "cluster-agent": CLUSTER_AGENT_TAGS,
        "cluster-agent-cloudfoundry": CLUSTER_AGENT_CLOUDFOUNDRY_TAGS,
        "dogstatsd": DOGSTATSD_TAGS,
        "process-agent": PROCESS_AGENT_TAGS,
        "security-agent": SECURITY_AGENT_TAGS,
        "system-probe": SYSTEM_PROBE_TAGS,
        "trace-agent": TRACE_AGENT_TAGS,
        # Test setups
        "test": TEST_TAGS,
        "test-with-process-tags": TEST_TAGS.union(PROCESS_AGENT_TAGS),
    },
    AgentFlavor.heroku: {
        "agent": AGENT_HEROKU_TAGS,
        "process-agent": PROCESS_AGENT_HEROKU_TAGS,
        "trace-agent": TRACE_AGENT_HEROKU_TAGS,
    },
    AgentFlavor.iot: {
        "agent": IOT_AGENT_TAGS,
    },
}


def get_default_build_tags(build="agent", arch="x64", flavor=AgentFlavor.base):
    """
    Build the default list of tags based on the build type and current platform.

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """
    include = build_tags.get(flavor).get(build)
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
        print(f"Warning: unknown build tag '{tag}' was filtered out from included tags list.")

    known_exclude = ALL_TAGS.intersection(exclude)
    unknown_exclude = exclude - known_exclude
    for tag in unknown_exclude:
        print(f"Warning: unknown build tag '{tag}' was filtered out from excluded tags list.")

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
