# (C) Datadog, Inc. 2010-2017
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
import logging
import os

# 3p
import simplejson as json

# project


log = logging.getLogger(__name__)


def load_manifest(path):
    manifest = None
    try:
        if path and os.path.exists(path):
            with open(path) as fp:
                manifest = json.load(fp)
    except (IOError, json.JSONDecodeError) as e:
        log.warn("Unable to read manifest at %s : %s", path, e)
        manifest = {}

    return manifest
