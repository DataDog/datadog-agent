# stdlib
import re
import urlparse

# 3rd party
import requests

# project
from checks import AgentCheck
from util import headers

VERSION_REGEX = re.compile(r".*/(\d)")


class Lighttpd(AgentCheck):
    """Tracks basic connection/requests/workers metrics

    See http://redmine.lighttpd.net/projects/1/wiki/Docs_ModStatus for Lighttpd details
    See http://redmine.lighttpd.net/projects/lighttpd2/wiki/Mod_status for Lighttpd2 details
    """

    SERVICE_CHECK_NAME = 'lighttpd.can_connect'

    URL_SUFFIX_PER_VERSION = {
        1: '?auto',
        2: '?format=plain',
        'Unknown': '?auto'
    }

    GAUGES = {
        'IdleServers': 'lighttpd.performance.idle_server',
        'BusyServers': 'lighttpd.performance.busy_servers',
        'Uptime': 'lighttpd.performance.uptime',
        'Total kBytes': 'lighttpd.net.bytes',
        'Total Accesses': 'lighttpd.net.hits',
        'memory_usage': 'lighttpd.performance.memory_usage',
        'requests_avg': 'lighttpd.net.requests_avg',
        'traffic_out_avg': 'lighttpd.net.bytes_out_avg',
        'traffic_in_avg': 'lighttpd.net.bytes_in_avg',
        'connections_avg': 'lighttpd.net.connections_avg',
        'connection_state_start': 'lighttpd.connections.state_start',
        'connection_state_read_header': 'lighttpd.connections.state_read_header',
        'connection_state_handle_request': 'lighttpd.connections.state_handle_request',
        'connection_state_write_response': 'lighttpd.connections.state_write_response',
        'connection_state_keep_alive': 'lighttpd.connections.state_keep_alive',
        'requests_avg_5sec': 'lighttpd.net.requests_avg_5sec',
        'traffic_out_avg_5sec': 'lighttpd.net.bytes_out_avg_5sec',
        'traffic_in_avg_5sec': 'lighttpd.net.bytes_in_avg_5sec',
        'connections_avg_5sec': 'lighttpd.net.connections_avg_5sec',
    }

    COUNTERS = {
        'requests_abs': 'lighttpd.net.requests_total',
        'traffic_out_abs': 'lighttpd.net.bytes_out',
        'traffic_in_abs': 'lighttpd.net.bytes_in',
        'connections_abs': 'lighttpd.net.connections_total',
        'status_1xx': 'lighttpd.response.status_1xx',
        'status_2xx': 'lighttpd.response.status_2xx',
        'status_3xx': 'lighttpd.response.status_3xx',
        'status_4xx': 'lighttpd.response.status_4xx',
        'status_5xx': 'lighttpd.response.status_5xx',
    }

    RATES = {
        'Total kBytes': 'lighttpd.net.bytes_per_s',
        'Total Accesses': 'lighttpd.net.request_per_s'
    }

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self.assumed_url = {}

    def check(self, instance):
        if 'lighttpd_status_url' not in instance:
            raise Exception("Missing 'lighttpd_status_url' variable in Lighttpd config")

        url = self.assumed_url.get(instance['lighttpd_status_url'], instance['lighttpd_status_url'])

        tags = instance.get('tags', [])
        self.log.debug("Connecting to %s" % url)

        auth = None
        if 'user' in instance and 'password' in instance:
            auth = (instance['user'], instance['password'])

        # Submit a service check for status page availability.
        parsed_url = urlparse.urlparse(url)
        lighttpd_url = parsed_url.hostname
        lighttpd_port = parsed_url.port or 80
        service_check_tags = ['host:%s' % lighttpd_url, 'port:%s' % lighttpd_port]
        try:
            r = requests.get(url, auth=auth, headers=headers(self.agentConfig))
            r.raise_for_status()
        except Exception:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                               tags=service_check_tags)
            raise
        else:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK,
                               tags=service_check_tags)

        headers_resp = r.headers
        server_version = self._get_server_version(headers_resp)
        response = r.content

        metric_count = 0
        # Loop through and extract the numerical values
        for line in response.split('\n'):
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

                # Send metric as a counter, if applicable
                if metric in self.COUNTERS:
                    metric_count += 1
                    metric_name = self.COUNTERS[metric]
                    self.increment(metric_name, value, tags=tags)

        if metric_count == 0:
            url_suffix = self.URL_SUFFIX_PER_VERSION[server_version]
            if self.assumed_url.get(instance['lighttpd_status_url']) is None and url[-len(url_suffix):] != url_suffix:
                self.assumed_url[instance['lighttpd_status_url']] = '%s%s' % (url, url_suffix)
                self.warning("Assuming url was not correct. Trying to add %s suffix to the url" % url_suffix)
                self.check(instance)
            else:
                raise Exception("No metrics were fetched for this instance. Make sure "
                                "that %s is the proper url." % instance['lighttpd_status_url'])

    def _get_server_version(self, headers):
        server_version = headers.get("server", "")

        match = VERSION_REGEX.match(server_version)
        if match is None:
            self.log.debug("Lighttpd server version is Unknown")
            return "Unknown"

        version = int(match.group(1))
        self.log.debug("Lighttpd server version is %s" % version)
        return version
