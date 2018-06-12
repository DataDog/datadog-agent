# (C) Datadog, Inc. 2010-2017
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# stdlib
import urlparse

# 3rd party
import requests

# project
from datadog_checks.checks import AgentCheck
# compatability layer
try:
    from config import _is_affirmative
except ImportError:
    from datadog_checks.config import _is_affirmative

# compatability layer
try:
    from util import headers
except ImportError:
    from datadog_checks.utils.headers import headers


class Apache(AgentCheck):
    """Tracks basic connection/requests/workers metrics

    See http://httpd.apache.org/docs/2.2/mod/mod_status.html for more details
    """
    GAUGES = {
        'IdleWorkers': 'apache.performance.idle_workers',
        'BusyWorkers': 'apache.performance.busy_workers',
        'CPULoad': 'apache.performance.cpu_load',
        'Uptime': 'apache.performance.uptime',
        'Total kBytes': 'apache.net.bytes',
        'Total Accesses': 'apache.net.hits',
        'ConnsTotal': 'apache.conns_total',
        'ConnsAsyncWriting': 'apache.conns_async_writing',
        'ConnsAsyncKeepAlive': 'apache.conns_async_keep_alive',
        'ConnsAsyncClosing': 'apache.conns_async_closing'
    }

    RATES = {
        'Total kBytes': 'apache.net.bytes_per_s',
        'Total Accesses': 'apache.net.request_per_s'
    }

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.assumed_url = {}

    def check(self, instance):
        if 'apache_status_url' not in instance:
            raise Exception("Missing 'apache_status_url' in Apache config")

        url = self.assumed_url.get(instance['apache_status_url'], instance['apache_status_url'])

        connect_timeout = int(instance.get('connect_timeout', 5))
        receive_timeout = int(instance.get('receive_timeout', 15))

        tags = instance.get('tags', [])

        disable_ssl_validation = _is_affirmative(instance.get('disable_ssl_validation', False))

        auth = None
        if 'apache_user' in instance and 'apache_password' in instance:
            auth = (instance['apache_user'], instance['apache_password'])

        # Submit a service check for status page availability.
        parsed_url = urlparse.urlparse(url)
        apache_host = parsed_url.hostname
        apache_port = parsed_url.port or 80
        service_check_name = 'apache.can_connect'
        service_check_tags = ['host:%s' % apache_host, 'port:%s' % apache_port] + tags
        try:
            self.log.debug('apache check initiating request, connect timeout %d receive %d' %
                           (connect_timeout, receive_timeout))
            r = requests.get(url, auth=auth, headers=headers(self.agentConfig),
                             verify=not disable_ssl_validation, timeout=(connect_timeout, receive_timeout))
            r.raise_for_status()

        except Exception as e:
            self.log.warning("Caught exception %s" % str(e))
            self.service_check(service_check_name, AgentCheck.CRITICAL,
                               tags=service_check_tags)
            raise
        else:
            self.service_check(service_check_name, AgentCheck.OK,
                               tags=service_check_tags)
        self.log.debug("apache check succeeded")
        response = r.content
        metric_count = 0
        # Loop through and extract the numerical values
        for line in response.splitlines():
            values = line.split(': ')
            if len(values) == 2:  # match
                metric, value = values
                try:
                    value = float(value)
                except ValueError:
                    continue

                # Special case: kBytes => bytes
                if metric == 'Total kBytes':
                    value = value * 1024

                # Send metric as a gauge, if applicable
                if metric in self.GAUGES:
                    metric_count += 1
                    metric_name = self.GAUGES[metric]
                    self.gauge(metric_name, value, tags=tags)

                # Send metric as a rate, if applicable
                if metric in self.RATES:
                    metric_count += 1
                    metric_name = self.RATES[metric]
                    self.rate(metric_name, value, tags=tags)

        if metric_count == 0:
            if self.assumed_url.get(instance['apache_status_url'], None) is None and url[-5:] != '?auto':
                self.assumed_url[instance['apache_status_url']] = '%s?auto' % url
                self.warning("Assuming url was not correct. Trying to add ?auto suffix to the url")
                self.check(instance)
            else:
                raise Exception((
                    "No metrics were fetched for this instance. "
                    "Make sure that %s is the proper url.")
                     % instance['apache_status_url'])
