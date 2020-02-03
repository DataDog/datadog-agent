"""
Utilities to manage build tags
"""
import sys
import distro
from invoke import task

# ALL_TAGS lists any available build tag
ALL_TAGS = set([
    "apm",
    "clusterchecks",
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
    "netcgo", # Force the use of the CGO resolver. This will also have the effect of making the binary non-static
    "orchestrator",
    "process",
    "python",
    "secrets",
    "systemd",
    "zk",
    "zlib",
])

# PUPPY_TAGS lists the tags needed when building the Puppy Agent
PUPPY_TAGS = set([
    "zlib",
])

PROCESS_ONLY_TAGS = [
    "orchestrator",
]

LINUX_ONLY_TAGS = [
    "containerd",
    "cri",
    "kubeapiserver",
    "kubelet",
    "netcgo",
    "orchestrator",
]

REDHAT_AND_DEBIAN_ONLY_TAGS = [
    "systemd",
]

REDHAT_AND_DEBIAN_DIST = [
    'centos',
    'debian',
    'rhel',
    'ubuntu',
]


def get_default_build_tags(puppy=False, process=False):
    """
    Build the default list of tags based on the current platform.

    The container integrations are currently only supported on Linux, disabling on
    the Windows and Darwin builds.
    """
    if puppy:
        return PUPPY_TAGS

    include = ["all"]
    exclude = [] if sys.platform.startswith('linux') else LINUX_ONLY_TAGS
    # if not process agent, ignore process only tags
    if not process:
        exclude = exclude + PROCESS_ONLY_TAGS

    # remove all tags that are only available on debian distributions
    distname = distro.id().lower()
    if distname not in REDHAT_AND_DEBIAN_DIST:
        exclude = exclude + REDHAT_AND_DEBIAN_ONLY_TAGS

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

    tags_to_audit = ALL_TAGS.difference(set(build_exclude)).difference(set(PUPPY_TAGS))

    max_size = _compute_build_size(ctx, build_exclude=','.join(build_exclude))
    print("size with all tags is {} kB".format(max_size / 1000))

    puppy_size = _compute_build_size(ctx, puppy=True)
    print("puppy size is {} kB\n".format(puppy_size / 1000))

    report = {"unaccounted": max_size - puppy_size, "puppy": puppy_size}

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


def _compute_build_size(ctx, build_exclude=None, puppy=False):
    import os
    from .agent import build as agent_build
    agent_build(ctx, build_exclude=build_exclude, skip_assets=True, puppy=puppy)

    statinfo = os.stat('bin/agent/agent')
    return statinfo.st_size
