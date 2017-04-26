# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

import requests


DEFAULT_TIMEOUT = 10


def retrieve_json(url, timeout=DEFAULT_TIMEOUT, verify=True):
    r = requests.get(url, timeout=timeout, verify=verify)
    r.raise_for_status()
    return r.json()
