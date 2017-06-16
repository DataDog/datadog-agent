# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

def freeze(o):
    """
    Freezes any mutable object including dictionaries and lists for hashing.
    Accepts nested dictionaries.
    """
    if isinstance(o, dict):
        return frozenset(dict([(k, freeze(v)) for k,v in o.iteritems()]).iteritems())

    if isinstance(o, list):
        return tuple([freeze(v) for v in o])

    return o

def hash_mutable(m):
    return hash(freeze(m))
