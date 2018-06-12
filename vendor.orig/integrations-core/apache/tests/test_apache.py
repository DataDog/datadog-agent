# (C) Datadog, Inc. 2010-2018
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

import pytest
import os
import subprocess
import requests
import time
import logging

from datadog_checks.apache import Apache
from datadog_checks.utils.common import get_docker_hostname

log = logging.getLogger('test_apache')

CHECK_NAME = 'apache'

HERE = os.path.dirname(os.path.abspath(__file__))
HOST = get_docker_hostname()
PORT = '18180'
BASE_URL = "http://{0}:{1}".format(HOST, PORT)

STATUS_URL = "{0}/server-status".format(BASE_URL)
AUTO_STATUS_URL = "{0}?auto".format(STATUS_URL)

STATUS_CONFIG = {
    'apache_status_url': STATUS_URL,
    'tags': ['instance:first']
}

AUTO_CONFIG = {
    'apache_status_url': AUTO_STATUS_URL,
    'tags': ['instance:second']
}

BAD_CONFIG = {
    'apache_status_url': 'http://localhost:1234/server-status',
}

APACHE_GAUGES = [
    'apache.performance.idle_workers',
    'apache.performance.busy_workers',
    'apache.performance.cpu_load',
    'apache.performance.uptime',
    'apache.net.bytes',
    'apache.net.hits',
    'apache.conns_total',
    'apache.conns_async_writing',
    'apache.conns_async_keep_alive',
    'apache.conns_async_closing'
]

APACHE_RATES = [
    'apache.net.bytes_per_s',
    'apache.net.request_per_s'
]


def wait_for_apache():
    for _ in xrange(0, 100):
        res = None
        try:
            res = requests.get(STATUS_URL)
            res.raise_for_status
            return
        except Exception as e:
            log.info("exception: {0} res: {1}".format(e, res))
            time.sleep(2)
    raise Exception("Cannot start up apache")


@pytest.fixture(scope="session")
def spin_up_apache():
    env = os.environ
    env['APACHE_CONFIG'] = os.path.join(HERE, 'compose', 'httpd.conf')
    env['APACHE_DOCKERFILE'] = os.path.join(HERE, 'compose', 'Dockerfile')
    args = [
        "docker-compose",
        "-f", os.path.join(HERE, 'compose', 'apache.yaml')
    ]
    subprocess.check_call(args + ["up", "-d", "--build"], env=env)
    wait_for_apache()
    for _ in xrange(0, 100):
        requests.get(BASE_URL)
    time.sleep(20)
    yield
    subprocess.check_call(args + ["down"], env=env)


@pytest.fixture
def aggregator():
    from datadog_checks.stubs import aggregator
    aggregator.reset()
    return aggregator


def test_connection_failure(aggregator, spin_up_apache):
    apache_check = Apache(CHECK_NAME, {}, {})
    with pytest.raises(Exception):
        apache_check.check(BAD_CONFIG)

    assert aggregator.service_checks('apache.can_connect')[0].status == Apache.CRITICAL
    assert len(aggregator._metrics) == 0


def test_check(aggregator, spin_up_apache):
    apache_check = Apache(CHECK_NAME, {}, {})
    apache_check.check(STATUS_CONFIG)

    tags = STATUS_CONFIG['tags']
    for mname in APACHE_GAUGES + APACHE_RATES:
        aggregator.assert_metric(mname, tags=tags, count=1)
    assert aggregator.service_checks('apache.can_connect')[0].status == Apache.OK

    sc_tags = ['host:' + HOST, 'port:' + PORT] + tags
    for sc in aggregator.service_checks('apache.can_connect'):
        for tag in sc.tags:
            assert tag in sc_tags

    assert aggregator.metrics_asserted_pct == 100.0


def test_check_auto(aggregator, spin_up_apache):
    apache_check = Apache(CHECK_NAME, {}, {})
    apache_check.check(AUTO_CONFIG)

    tags = AUTO_CONFIG['tags']
    for mname in APACHE_GAUGES + APACHE_RATES:
        aggregator.assert_metric(mname, tags=tags, count=1)
    assert aggregator.service_checks('apache.can_connect')[0].status == Apache.OK

    sc_tags = ['host:' + HOST, 'port:' + PORT] + tags
    for sc in aggregator.service_checks('apache.can_connect'):
        for tag in sc.tags:
            assert tag in sc_tags

    assert aggregator.metrics_asserted_pct == 100.0
    aggregator.reset()
