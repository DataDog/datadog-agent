# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

# (C) Datadog, Inc. 2010-2016
# All rights reserved

import logging
from urlparse import urlparse

import datadog_agent


log = logging.getLogger(__name__)


def get_requests_proxy(agentConfig):
    no_proxy_settings = {
        "http": None,
        "https": None,
        "no": [],
    }

    config = {} if agentConfig is None else agentConfig

    # First we read the proxy configuration from datadog.conf
    proxies = config.get('proxy', datadog_agent.get_config('proxy'))
    if proxies:
        proxies = proxies.copy()

    # requests compliant dict
    if proxies and 'no_proxy' in proxies:
        proxies['no'] = proxies.pop('no_proxy')

    return proxies if proxies else no_proxy_settings


def config_proxy_skip(proxies, uri, skip_proxy=False):
    """
    Returns an amended copy of the proxies dictionary - used by `requests`,
    it will disable the proxy if the uri provided is to be reached directly.

    Keyword Arguments:
        proxies -- dict with existing proxies: 'https', 'http', 'no' as pontential keys
        uri -- uri to determine if proxy is necessary or not.
        skip_proxy -- if True, the proxy dictionary returned will disable all proxies
    """
    parsed_uri = urlparse(uri)

    # disable proxy if necessary
    if skip_proxy:
        if 'http' in proxies:
            proxies.pop('http')
        if 'https' in proxies:
            proxies.pop('https')
    elif proxies.get('no'):
        urls = []
        if isinstance(proxies['no'], basestring):
            urls = proxies['no'].replace(';', ',').split(",")
        elif isinstance(proxies['no'], list):
            urls = proxies['no']
        for url in urls:
            if url in parsed_uri.netloc:
                if 'http' in proxies:
                    proxies.pop('http')
                if 'https' in proxies:
                    proxies.pop('https')

    return proxies
