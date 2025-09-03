# (C) Datadog, Inc. 2018-present
# All rights reserved
# Licensed under a 3-clause BSD style license (see LICENSE)
from collections import namedtuple

from requests.structures import CaseInsensitiveDict

from datadog_checks.base import ConfigurationError, ensure_unicode, is_affirmative
from datadog_checks.base.utils.headers import headers as agent_headers

DEFAULT_EXPECTED_CODE = r'(1|2|3)\d\d'


Config = namedtuple(
    'Config',
    [
        'url',
        'client_cert',
        'client_key',
        'method',
        'data',
        'http_response_status_code',
        'include_content',
        'headers',
        'response_time',
        'content_match',
        'reverse_content_match',
        'tags',
        'ssl_expire',
        'instance_ca_certs',
        'check_hostname',
        'stream',
        'use_cert_from_response',
    ],
)


def from_instance(instance, default_ca_certs=None):
    """
    Create a config object from an instance dictionary
    """
    method = instance.get('method', 'get')
    data = instance.get('data', {})
    tags = instance.get('tags', [])
    client_cert = instance.get('tls_cert') or instance.get('client_cert')
    client_key = instance.get('tls_private_key') or instance.get('client_key')
    http_response_status_code = str(instance.get('http_response_status_code', DEFAULT_EXPECTED_CODE))
    config_headers = instance.get('headers', {})
    default_headers = is_affirmative(instance.get("include_default_headers", True))
    if default_headers:
        headers = CaseInsensitiveDict(agent_headers({}))
    else:
        headers = CaseInsensitiveDict({})
    headers.update(config_headers)
    url = instance.get('url')
    if url is not None:
        url = ensure_unicode(url)
    content_match = instance.get('content_match')
    if content_match is not None:
        content_match = ensure_unicode(content_match)
    reverse_content_match = is_affirmative(instance.get('reverse_content_match', False))
    response_time = is_affirmative(instance.get('collect_response_time', True))
    if not url:
        raise ConfigurationError("Bad configuration. You must specify a url")
    if not url.startswith("http"):
        raise ConfigurationError("The url {} must start with the scheme http or https".format(url))
    include_content = is_affirmative(instance.get('include_content', False))
    ssl_expire = is_affirmative(instance.get('check_certificate_expiration', True))
    instance_ca_certs = instance.get('tls_ca_cert', instance.get('ca_certs', default_ca_certs))
    check_hostname = is_affirmative(instance.get('check_hostname', True))
    stream = is_affirmative(instance.get('stream', False))
    use_cert_from_response = is_affirmative(instance.get('use_cert_from_response', False))
    if use_cert_from_response:
        stream = True

    return Config(
        url,
        client_cert,
        client_key,
        method,
        data,
        http_response_status_code,
        include_content,
        headers,
        response_time,
        content_match,
        reverse_content_match,
        tags,
        ssl_expire,
        instance_ca_certs,
        check_hostname,
        stream,
        use_cert_from_response,
    )
