"""
Utilities to manage build tags
"""
import invoke


# ALL_TAGS lists any available build tag
ALL_TAGS = set([
    "apm",
    "consul",
    "cpython",
    "docker",
    "ec2",
    "etcd",
    "gce",
    "jmx",
    "kubelet",
    "log",
    "process",
    "snmp",
    "zk",
    "zlib",
])

# PUPPY_TAGS lists the tags needed when building the Puppy Agent
PUPPY_TAGS = set([
    "zlib",
])


def get_default_build_tags(puppy=False):
    """
    Build the default list of tags based on the current platform.
    """
    if puppy:
        return PUPPY_TAGS

    include = ["all"]
    exclude = ["docker"] if invoke.platform.WINDOWS else []
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
