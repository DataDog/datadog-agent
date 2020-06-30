"""
Utilities to manage build tags
"""
import sys
from invoke import task

# ALL_TAGS lists any available build tag
ALL_TAGS = set(
    [
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

# IOT_AGENT_TAGS lists the tags needed when building the IOT Agent
IOT_AGENT_TAGS = [
    "zlib",
    "systemd",
]

ANDROID_TAGS = [
    "zlib",
    "android",
]

PROCESS_ONLY_TAGS = [
    "fargateprocess",
    "orchestrator",
]

LINUX_ONLY_TAGS = [
    "containerd",
    "cri",
    "netcgo",
    "systemd",
]
WINDOWS_32BIT_EXCLUDE_TAGS = [
    "orchestrator",
    "docker",
    "kubeapiserver",
    "kubelet",
]


def get_default_build_tags(iot=False, process=False, arch="x64", android=False):
    """
    Build the default list of tags based on the current platform.

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """
    include = ["all"]
    if iot:
        include = IOT_AGENT_TAGS

    # android has its own set of tags
    if android:
        include = ANDROID_TAGS

    exclude = [] if sys.platform.startswith("linux") else LINUX_ONLY_TAGS
    # if not process agent, ignore process only tags
    if not process:
        exclude = exclude + PROCESS_ONLY_TAGS

    # Force exclusion of Windows 32bits tag
    if sys.platform == "win32" and arch == "x86":
        exclude = exclude + WINDOWS_32BIT_EXCLUDE_TAGS

    return get_build_tags(include, exclude)


def get_build_tags(include, exclude):
    """
    Build the list of tags based on inclusions and exclusions passed through
    the command line
    """
    # special case, include == all
    if "all" in include:
        return list(ALL_TAGS - set(exclude))

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
