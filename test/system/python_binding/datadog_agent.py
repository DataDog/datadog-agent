# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

import sys
import socket
import nose
from nose.tools import assert_equals
import datadog_agent


def test_agent_verion():
    assert_equals(datadog_agent.get_version(), "6.0.0")

def test_get_config():
    assert_equals(datadog_agent.get_config('dd_url'), "https://test.datadoghq.com")

def test_headers():
    assert_equals(datadog_agent.headers(), {'Content-Type': 'application/x-www-form-urlencoded', 'Accept': 'text/html, */*', 'User-Agent': 'Datadog Agent/6.0.0'})

def test_get_hostname():
    assert_equals(datadog_agent.get_hostname(), "test.hostname")

if __name__ == '__main__':
    if not nose.run(defaultTest=__name__):
        sys.exit(1)
