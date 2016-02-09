# stdlib
from collections import defaultdict
import re
import time

# 3rd party
import requests

# project
from checks import AgentCheck
from config import _is_affirmative
from util import headers

STATS_URL = "/;csv;norefresh"
EVENT_TYPE = SOURCE_TYPE_NAME = 'haproxy'


class Services(object):
    BACKEND = 'BACKEND'
    FRONTEND = 'FRONTEND'
    ALL = (BACKEND, FRONTEND)
    ALL_STATUSES = (
        'up', 'open', 'no check', 'down', 'maint', 'nolb'
    )
    STATUSES_TO_SERVICE_CHECK = {
        'UP': AgentCheck.OK,
        'DOWN': AgentCheck.CRITICAL,
        'no check': AgentCheck.UNKNOWN,
        'MAINT': AgentCheck.OK,
    }


class HAProxy(AgentCheck):
    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)

        # Host status needs to persist across all checks
        self.host_status = defaultdict(lambda: defaultdict(lambda: None))

    METRICS = {
        "qcur": ("gauge", "queue.current"),
        "scur": ("gauge", "session.current"),
        "slim": ("gauge", "session.limit"),
        "spct": ("gauge", "session.pct"),    # Calculated as: (scur/slim)*100
        "stot": ("rate", "session.rate"),
        "bin": ("rate", "bytes.in_rate"),
        "bout": ("rate", "bytes.out_rate"),
        "dreq": ("rate", "denied.req_rate"),
        "dresp": ("rate", "denied.resp_rate"),
        "ereq": ("rate", "errors.req_rate"),
        "econ": ("rate", "errors.con_rate"),
        "eresp": ("rate", "errors.resp_rate"),
        "wretr": ("rate", "warnings.retr_rate"),
        "wredis": ("rate", "warnings.redis_rate"),
        "req_rate": ("gauge", "requests.rate"), # HA Proxy 1.4 and higher
        "hrsp_1xx": ("rate", "response.1xx"),  # HA Proxy 1.4 and higher
        "hrsp_2xx": ("rate", "response.2xx"), # HA Proxy 1.4 and higher
        "hrsp_3xx": ("rate", "response.3xx"), # HA Proxy 1.4 and higher
        "hrsp_4xx": ("rate", "response.4xx"), # HA Proxy 1.4 and higher
        "hrsp_5xx": ("rate", "response.5xx"), # HA Proxy 1.4 and higher
        "hrsp_other": ("rate", "response.other"), # HA Proxy 1.4 and higher
        "qtime": ("gauge", "queue.time"),  # HA Proxy 1.5 and higher
        "ctime": ("gauge", "connect.time"),  # HA Proxy 1.5 and higher
        "rtime": ("gauge", "response.time"),  # HA Proxy 1.5 and higher
        "ttime": ("gauge", "session.time"),  # HA Proxy 1.5 and higher
    }

    SERVICE_CHECK_NAME = 'haproxy.backend_up'

    def check(self, instance):
        url = instance.get('url')
        username = instance.get('username')
        password = instance.get('password')
        collect_aggregates_only = _is_affirmative(
            instance.get('collect_aggregates_only', True)
        )
        collect_status_metrics = _is_affirmative(
            instance.get('collect_status_metrics', False)
        )
        collect_status_metrics_by_host = _is_affirmative(
            instance.get('collect_status_metrics_by_host', False)
        )
        tag_service_check_by_host = _is_affirmative(
            instance.get('tag_service_check_by_host', False)
        )
        services_incl_filter = instance.get('services_include', [])
        services_excl_filter = instance.get('services_exclude', [])

        self.log.debug('Processing HAProxy data for %s' % url)

        data = self._fetch_data(url, username, password)

        process_events = instance.get('status_check', self.init_config.get('status_check', False))

        self._process_data(
            data, collect_aggregates_only, process_events,
            url=url, collect_status_metrics=collect_status_metrics,
            collect_status_metrics_by_host=collect_status_metrics_by_host,
            tag_service_check_by_host=tag_service_check_by_host,
            services_incl_filter=services_incl_filter,
            services_excl_filter=services_excl_filter
        )

    def _fetch_data(self, url, username, password):
        ''' Hit a given URL and return the parsed json '''
        # Try to fetch data from the stats URL

        auth = (username, password)
        url = "%s%s" % (url, STATS_URL)

        self.log.debug("HAProxy Fetching haproxy search data from: %s" % url)

        r = requests.get(url, auth=auth, headers=headers(self.agentConfig))
        r.raise_for_status()

        return r.content.splitlines()

    def _process_data(self, data, collect_aggregates_only, process_events, url=None,
                      collect_status_metrics=False, collect_status_metrics_by_host=False,
                      tag_service_check_by_host=False, services_incl_filter=None,
                      services_excl_filter=None):
        ''' Main data-processing loop. For each piece of useful data, we'll
        either save a metric, save an event or both. '''

        # Split the first line into an index of fields
        # The line looks like:
        # "# pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,"
        fields = [f.strip() for f in data[0][2:].split(',') if f]

        self.hosts_statuses = defaultdict(int)

        back_or_front = None

        # Skip the first line, go backwards to set back_or_front
        for line in data[:0:-1]:
            if not line.strip():
                continue

            # Store each line's values in a dictionary
            data_dict = self._line_to_dict(fields, line)

            if self._is_aggregate(data_dict):
                back_or_front = data_dict['svname']

            self._update_data_dict(data_dict, back_or_front)

            self._update_hosts_statuses_if_needed(
                collect_status_metrics, collect_status_metrics_by_host,
                data_dict, self.hosts_statuses
            )

            if self._should_process(data_dict, collect_aggregates_only):
                # update status
                # Send the list of data to the metric and event callbacks
                self._process_metrics(
                    data_dict, url,
                    services_incl_filter=services_incl_filter,
                    services_excl_filter=services_excl_filter
                )
            if process_events:
                self._process_event(
                    data_dict, url,
                    services_incl_filter=services_incl_filter,
                    services_excl_filter=services_excl_filter
                )
            self._process_service_check(
                data_dict, url,
                tag_by_host=tag_service_check_by_host,
                services_incl_filter=services_incl_filter,
                services_excl_filter=services_excl_filter
            )

        if collect_status_metrics:
            self._process_status_metric(
                self.hosts_statuses, collect_status_metrics_by_host,
                services_incl_filter=services_incl_filter,
                services_excl_filter=services_excl_filter
            )
            self._process_backend_hosts_metric(
                self.hosts_statuses,
                services_incl_filter=services_incl_filter,
                services_excl_filter=services_excl_filter
            )

        return data

    def _line_to_dict(self, fields, line):
        data_dict = {}
        for i, val in enumerate(line.split(',')[:]):
            if val:
                try:
                    # Try converting to a long, if failure, just leave it
                    val = float(val)
                except Exception:
                    pass
                data_dict[fields[i]] = val
        return data_dict

    def _update_data_dict(self, data_dict, back_or_front):
        """
        Adds spct if relevant, adds service
        """
        data_dict['back_or_front'] = back_or_front
        # The percentage of used sessions based on 'scur' and 'slim'
        if 'slim' in data_dict and 'scur' in data_dict:
            try:
                data_dict['spct'] = (data_dict['scur'] / data_dict['slim']) * 100
            except (TypeError, ZeroDivisionError):
                pass

    def _is_aggregate(self, data_dict):
        return data_dict['svname'] in Services.ALL

    def _update_hosts_statuses_if_needed(self, collect_status_metrics,
                                         collect_status_metrics_by_host,
                                         data_dict, hosts_statuses):
        if data_dict['svname'] == Services.BACKEND:
            return
        if collect_status_metrics and 'status' in data_dict and 'pxname' in data_dict:
            if collect_status_metrics_by_host and 'svname' in data_dict:
                key = (data_dict['pxname'], data_dict['svname'], data_dict['status'])
            else:
                key = (data_dict['pxname'], data_dict['status'])
            hosts_statuses[key] += 1

    def _should_process(self, data_dict, collect_aggregates_only):
        """
            if collect_aggregates_only, we process only the aggregates
            else we process all except Services.BACKEND
        """
        if collect_aggregates_only:
            if self._is_aggregate(data_dict):
                return True
            return False
        elif data_dict['svname'] == Services.BACKEND:
            return False
        return True

    def _is_service_excl_filtered(self, service_name, services_incl_filter,
                                  services_excl_filter):
        if self._tag_match_patterns(service_name, services_excl_filter):
            if self._tag_match_patterns(service_name, services_incl_filter):
                return False
            return True
        return False

    def _tag_match_patterns(self, tag, filters):
        if not filters:
            return False
        for rule in filters:
            if re.search(rule, tag):
                return True
        return False

    def _process_backend_hosts_metric(self, hosts_statuses, services_incl_filter=None,
                                      services_excl_filter=None):
        agg_statuses = defaultdict(lambda: {'available': 0, 'unavailable': 0})
        for host_status, count in hosts_statuses.iteritems():
            try:
                service, hostname, status = host_status
            except Exception:
                service, status = host_status

            if self._is_service_excl_filtered(service, services_incl_filter, services_excl_filter):
                continue
            status = status.lower()
            if 'up' in status:
                agg_statuses[service]['available'] += count
            elif 'down' in status or 'maint' in status or 'nolb' in status:
                agg_statuses[service]['unavailable'] += count
            else:
                # create the entries for this service anyway
                agg_statuses[service]

        for service in agg_statuses:
            tags = ['service:%s' % service]
            self.gauge(
                'haproxy.backend_hosts',
                agg_statuses[service]['available'],
                tags=tags + ['available:true'])
            self.gauge(
                'haproxy.backend_hosts',
                agg_statuses[service]['unavailable'],
                tags=tags + ['available:false'])
        return agg_statuses

    def _process_status_metric(self, hosts_statuses, collect_status_metrics_by_host,
                               services_incl_filter=None, services_excl_filter=None):
        agg_statuses = defaultdict(lambda: {'available': 0, 'unavailable': 0})
        for host_status, count in hosts_statuses.iteritems():
            try:
                service, hostname, status = host_status
            except Exception:
                service, status = host_status
            status = status.lower()

            tags = ['service:%s' % service]
            if self._is_service_excl_filtered(service, services_incl_filter, services_excl_filter):
                continue

            if collect_status_metrics_by_host:
                tags.append('backend:%s' % hostname)
            self._gauge_all_statuses("haproxy.count_per_status", count, status, tags=tags)

            if 'up' in status or 'open' in status:
                agg_statuses[service]['available'] += count
            if 'down' in status or 'maint' in status or 'nolb' in status:
                agg_statuses[service]['unavailable'] += count

        for service in agg_statuses:
            for status, count in agg_statuses[service].iteritems():
                tags = ['status:%s' % status, 'service:%s' % service]
                self.gauge("haproxy.count_per_status", count, tags=tags)

    def _gauge_all_statuses(self, metric_name, count, status, tags):
        self.gauge(metric_name, count, tags + ['status:%s' % status])
        for state in Services.ALL_STATUSES:
            if state != status:
                self.gauge(metric_name, 0, tags + ['status:%s' % state.replace(" ", "_")])

    def _process_metrics(self, data, url, services_incl_filter=None,
                         services_excl_filter=None):
        """
        Data is a dictionary related to one host
        (one line) extracted from the csv.
        It should look like:
        {'pxname':'dogweb', 'svname':'i-4562165', 'scur':'42', ...}
        """
        hostname = data['svname']
        service_name = data['pxname']
        back_or_front = data['back_or_front']
        tags = ["type:%s" % back_or_front, "instance_url:%s" % url]
        tags.append("service:%s" % service_name)

        if self._is_service_excl_filtered(service_name, services_incl_filter,
                                          services_excl_filter):
            return

        if back_or_front == Services.BACKEND:
            tags.append('backend:%s' % hostname)

        for key, value in data.items():
            if HAProxy.METRICS.get(key):
                suffix = HAProxy.METRICS[key][1]
                name = "haproxy.%s.%s" % (back_or_front.lower(), suffix)
                if HAProxy.METRICS[key][0] == 'rate':
                    self.rate(name, value, tags=tags)
                else:
                    self.gauge(name, value, tags=tags)

    def _process_event(self, data, url, services_incl_filter=None,
                       services_excl_filter=None):
        '''
        Main event processing loop. An event will be created for a service
        status change.
        Service checks on the server side can be used to provide the same functionality
        '''
        hostname = data['svname']
        service_name = data['pxname']
        key = "%s:%s" % (hostname, service_name)
        status = self.host_status[url][key]

        if self._is_service_excl_filtered(service_name, services_incl_filter,
                                          services_excl_filter):
            return

        if status is None:
            self.host_status[url][key] = data['status']
            return

        if status != data['status'] and data['status'] in ('UP', 'DOWN'):
            # If the status of a host has changed, we trigger an event
            try:
                lastchg = int(data['lastchg'])
            except Exception:
                lastchg = 0

            # Create the event object
            ev = self._create_event(
                data['status'], hostname, lastchg, service_name,
                data['back_or_front']
            )
            self.event(ev)

            # Store this host status so we can check against it later
            self.host_status[url][key] = data['status']

    def _create_event(self, status, hostname, lastchg, service_name, back_or_front):
        HAProxy_agent = self.hostname.decode('utf-8')
        if status == "DOWN":
            alert_type = "error"
            title = "%s reported %s:%s %s" % (HAProxy_agent, service_name, hostname, status)
        else:
            if status == "UP":
                alert_type = "success"
            else:
                alert_type = "info"
            title = "%s reported %s:%s back and %s" % (HAProxy_agent, service_name, hostname, status)

        tags = ["service:%s" % service_name]
        if back_or_front == Services.BACKEND:
            tags.append('backend:%s' % hostname)
        return {
            'timestamp': int(time.time() - lastchg),
            'event_type': EVENT_TYPE,
            'host': HAProxy_agent,
            'msg_title': title,
            'alert_type': alert_type,
            "source_type_name": SOURCE_TYPE_NAME,
            "event_object": hostname,
            "tags": tags
        }

    def _process_service_check(self, data, url, tag_by_host=False,
                               services_incl_filter=None, services_excl_filter=None):
        ''' Report a service check, tagged by the service and the backend.
            Statuses are defined in `STATUSES_TO_SERVICE_CHECK` mapping.
        '''
        service_name = data['pxname']
        status = data['status']
        haproxy_hostname = self.hostname.decode('utf-8')
        check_hostname = haproxy_hostname if tag_by_host else ''

        if self._is_service_excl_filtered(service_name, services_incl_filter,
                                          services_excl_filter):
            return

        if status in Services.STATUSES_TO_SERVICE_CHECK:
            service_check_tags = ["service:%s" % service_name]
            hostname = data['svname']
            if data['back_or_front'] == Services.BACKEND:
                service_check_tags.append('backend:%s' % hostname)

            status = Services.STATUSES_TO_SERVICE_CHECK[status]
            message = "%s reported %s:%s %s" % (haproxy_hostname, service_name,
                                                hostname, status)
            self.service_check(self.SERVICE_CHECK_NAME, status,  message=message,
                               hostname=check_hostname, tags=service_check_tags)
