"""
Utilities to manage build tags
"""
import sys
from invoke import task

# ALL_TAGS lists all available build tags.
# Used to remove unknown tags from provided tag lists.
ALL_TAGS = set([
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
    "kubeapiserver",
    "kubelet",
    "linux_bpf",
    "netcgo", # Force the use of the CGO resolver. This will also have the effect of making the binary non-static
    "orchestrator",
    "process",
    "python",
    "secrets",
    "systemd",
    "zk",
    "zlib",
])

### Tag inclusion lists

# AGENT_TAGS lists the tags needed when building the agent.
AGENT_TAGS = [
    "apm",
    "consul",
    "containerd",
    "cri",
    "docker",
    "ec2",
    "etcd",
    "gce",
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

# ANDROID_TAGS lists the tags needed when building the android agent
ANDROID_TAGS = [
    "android",
    "zlib",
]

# CLUSTER_AGENT_TAGS lists the tags needed when building the cluster-agent
CLUSTER_AGENT_TAGS = [
    "clusterchecks",
    "kubeapiserver",
    "orchestrator",
    "secrets",
    "zlib",
]

# CLUSTER_AGENT_CLOUDFOUNDRY_TAGS lists the tags needed when building the cloudfoundry cluster-agent
CLUSTER_AGENT_CLOUDFOUNDRY_TAGS = [
    "clusterchecks",
    "secrets",
]

# DOGSTATSD_TAGS lists the tags needed when building dogstatsd
DOGSTATSD_TAGS = [
    "docker",
    "kubelet",
    "secrets",
    "zlib",
]

# IOT_AGENT_TAGS lists the tags needed when building the IoT agent
IOT_AGENT_TAGS = [
    "systemd",
    "zlib",
]

# PROCESS_AGENT_TAGS lists the tags necessary to build the process-agent
PROCESS_AGENT_TAGS = AGENT_TAGS + [
    "clusterchecks",
    "fargateprocess",
    "orchestrator",
]

# SECURITY_AGENT_TAGS lists the tags necessary to build the security agent
SECURITY_AGENT_TAGS = AGENT_TAGS + [
    "clusterchecks",
]

# PROCESS_AGENT_TAGS lists the tags necessary to build system-probe
SYSTEM_PROBE_TAGS = AGENT_TAGS + [
    "clusterchecks",
    "linux_bpf",
]

# TRACE_AGENT_TAGS lists the tags that have to be added when the trace-agent
TRACE_AGENT_TAGS = [
    "docker",
    "kubeapiserver",
    "kubelet",
    "netcgo",
    "secrets",
]

# TEST_TAGS lists the tags that have to be added to run tests
TEST_TAGS = AGENT_TAGS + [
    "clusterchecks",
]

### Tag exclusion lists

# List of tags to always remove when not building on Linux
LINUX_ONLY_TAGS = [
    "containerd",
    "cri",
    "netcgo",
    "systemd",
]

# List of tags to always remove when building on Windows
WINDOWS_EXCLUDE_TAGS = [
    "linux_bpf"
]

# List of tags to always remove when building on Windows 32-bits
WINDOWS_32BIT_EXCLUDE_TAGS = [
    "docker",
    "kubeapiserver",
    "kubelet",
    "orchestrator",
]

def get_default_build_tags(build="agent", arch="x64"):
    """
    Build the default list of tags based on the build type and current platform.

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """
    include = []
    # Build setups
    if build == "agent":
        include = AGENT_TAGS
    if build == "android":
        include = ANDROID_TAGS
    if build == "cluster-agent":
        include = CLUSTER_AGENT_TAGS
    if build == "cluster-agent-cloudfoundry":
        include = CLUSTER_AGENT_CLOUDFOUNDRY_TAGS
    if build == "dogstatsd":
        include = DOGSTATSD_TAGS
    if build == "iot":
        include = IOT_AGENT_TAGS
    if build == "process-agent":
        include = PROCESS_AGENT_TAGS
    if build == "security-agent":
        include = SECURITY_AGENT_TAGS
    if build == "system-probe":
        include = SYSTEM_PROBE_TAGS
    if build == "trace-agent":
        include = TRACE_AGENT_TAGS
    # Test setups
    if build == "test":
        include = TEST_TAGS
    if build == "test-with-process-tags":
        include = TEST_TAGS + PROCESS_AGENT_TAGS

    return filter_incorrect_tags(include, arch=arch)

def filter_incorrect_tags(include, arch="x64"):
    """
    Filter out tags incompatible with the platform.
    """

    exclude = []
    if not sys.platform.startswith("linux"):
        exclude += LINUX_ONLY_TAGS

    if sys.platform == "win32":
        exclude += WINDOWS_EXCLUDE_TAGS

    if sys.platform == "win32" and arch == "x86":
        exclude += WINDOWS_32BIT_EXCLUDE_TAGS

    return get_build_tags(include, exclude)


def get_build_tags(include, exclude):
    """
    Build the list of tags based on inclusions and exclusions passed through
    the command line
    """

    # filter out unrecognised tags
    include = ALL_TAGS.intersection(set(include))
    exclude = ALL_TAGS.intersection(set(exclude))
    return list(include - exclude)


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
        delta = (max_size - size)
        print("tag {} adds {} kB (excludes: {})".format(tag, delta / 1000, exclude_string))
        report[tag] = delta
        report["unaccounted"] -= delta

    if csv:
        print("\nCSV output in bytes:")
        for k, v in report.iteritems():
            print("{};{}".format(k, v))


def _compute_build_size(ctx, build_exclude=None, iot=False):
    import os
    from .agent import build as agent_build
    agent_build(ctx, build_exclude=build_exclude, skip_assets=True, iot=iot)

    statinfo = os.stat('bin/agent/agent')
    return statinfo.st_size
