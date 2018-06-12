# (C) Datadog, Inc. 2010-2018
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

import os
import pytest
import logging

import simplejson as json
from requests import Session, Response

from datadog_checks.cisco_aci import CiscoACICheck
from datadog_checks.cisco_aci.api import SessionWrapper, Api

from datadog_checks.utils.containers import hash_mutable

from .common import FIXTURE_LIST_FILE_MAP

log = logging.getLogger('test_cisco_aci')

CHECK_NAME = 'cisco_aci'

FIXTURES_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'fixtures')

USERNAME = 'datadog'
PASSWORD = 'datadog'
ACI_URL = 'https://datadoghq.com'
ACI_URLS = [ACI_URL]
CONFIG = {
    'aci_urls': ACI_URLS,
    'username': USERNAME,
    'pwd': PASSWORD,
    'tenant': [
        'DataDog',
    ]
}


class FakeSess(SessionWrapper):
    def make_request(self, path, raw_response=False):
        mock_path = path.replace('/', '_')
        mock_path = mock_path.replace('?', '_')
        mock_path = mock_path.replace('&', '_')
        mock_path = mock_path.replace('=', '_')
        mock_path = mock_path.replace(',', '_')
        mock_path = mock_path.replace('-', '_')
        mock_path = mock_path.replace('.', '_')
        mock_path = mock_path.replace('"', '_')
        mock_path = mock_path.replace('(', '_')
        mock_path = mock_path.replace(')', '_')
        mock_path = mock_path.replace('[', '_')
        mock_path = mock_path.replace(']', '_')
        mock_path = mock_path.replace('|', '_')
        mock_path = FIXTURE_LIST_FILE_MAP[mock_path]
        mock_path = os.path.join(FIXTURES_DIR, mock_path)
        mock_path += '.txt'

        log.info(os.listdir(FIXTURES_DIR))

        with open(mock_path, 'r') as f:
            return json.loads(f.read())


@pytest.fixture
def aggregator():
    from datadog_checks.stubs import aggregator
    aggregator.reset()
    return aggregator


def mock_send(prepped_request, **kwargs):
    if prepped_request.path_url == '/api/aaaLogin.xml':
        cookie_path = os.path.join(FIXTURES_DIR, 'login_cookie.txt')
        response_path = os.path.join(FIXTURES_DIR, 'login.txt')
        response = Response()
        with open(cookie_path, 'r') as f:
            response.cookies = {'APIC-cookie': f.read()}
        with open(response_path, 'r') as f:
            response.raw = f.read()

    return response


@pytest.fixture
def session_mock():
    session = Session()
    setattr(session, 'send', mock_send)
    fake_session_wrapper = FakeSess(ACI_URL, session, 'cookie')

    return fake_session_wrapper


def test_cisco(aggregator, session_mock):
    cisco_aci_check = CiscoACICheck(CHECK_NAME, {}, {})
    api = Api(ACI_URLS, USERNAME, PASSWORD, log=cisco_aci_check.log)
    api.sessions = [session_mock]
    api._refresh_sessions = False
    cisco_aci_check._api_cache[hash_mutable(CONFIG)] = api

    cisco_aci_check.check(CONFIG)
