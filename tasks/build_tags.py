"""
Utilities to manage build tags
"""

# ALL_TAGS lists any available build tag
ALL_TAGS = set([
    "zlib",
    "snmp",
    "etcd",
    "zk",
    "cpython",
    "jmx",
    "apm",
    "docker",
    "ec2",
    "gce",
    "process",
    "zk",
    "consul",
])


def get_build_tags(include=["all"], exclude=[]):
    """
    Build the list of tags based on inclusions and exclusions passed through
    the command line
    """
    # special case, include == all
    if 'all' in include:
        return list(ALL_TAGS - set(exclude))

    # filter out unrecognised tags
    include = ALL_TAGS.intersection(set(include))
    exclude = ALL_TAGS.intersection(set(exclude))
    return list(include - exclude)


def get_puppy_build_tags():
    """
    Return the list of tags composing the puppy version
    """
    return ["zlib"]
