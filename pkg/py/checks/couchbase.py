# stdlib
import re

# 3rd party
import requests

# project
from checks import AgentCheck
from util import headers

# Constants
COUCHBASE_STATS_PATH = '/pools/default'
DEFAULT_TIMEOUT = 10


class Couchbase(AgentCheck):
    """Extracts stats from Couchbase via its REST API
    http://docs.couchbase.com/couchbase-manual-2.0/#using-the-rest-api
    """
    SERVICE_CHECK_NAME = 'couchbase.can_connect'

    # Selected metrics to send amongst all the bucket stats, after name normalization
    BUCKET_STATS = set([
        'avg_bg_wait_time',
        'avg_disk_commit_time',
        'bytes_read',
        'bytes_written',
        'cas_hits',
        'cas_misses',
        'cmd_get',
        'cmd_set',
        'couch_docs_actual_disk_size',
        'couch_docs_data_size',
        'couch_docs_disk_size',
        'couch_docs_fragmentation',
        'couch_total_disk_size',
        'couch_views_fragmentation',
        'couch_views_ops',
        'cpu_idle_ms',
        'cpu_utilization_rate',
        'curr_connections',
        'curr_items',
        'curr_items_tot',
        'decr_hits',
        'decr_misses',
        'delete_hits',
        'delete_misses',
        'disk_commit_count',
        'disk_update_count',
        'disk_write_queue',
        'ep_bg_fetched',
        'ep_cache_miss_rate',
        'ep_cache_miss_ratio',
        'ep_diskqueue_drain',
        'ep_diskqueue_fill',
        'ep_flusher_todo',
        'ep_item_commit_failed',
        'ep_max_size',
        'ep_mem_high_wat',
        'ep_mem_low_wat',
        'ep_num_non_resident',
        'ep_num_value_ejects',
        'ep_oom_errors',
        'ep_ops_create',
        'ep_ops_update',
        'ep_overhead',
        'ep_queue_size',
        'ep_resident_items_rate',
        'ep_tap_replica_queue_drain',
        'ep_tap_total_queue_drain',
        'ep_tap_total_queue_fill',
        'ep_tap_total_total_backlog_size',
        'ep_tmp_oom_errors',
        'evictions',
        'get_hits',
        'get_misses',
        'hit_ratio',
        'incr_hits',
        'incr_misses',
        'mem_free',
        'mem_total',
        'mem_used',
        'misses',
        'ops',
        'page_faults',
        'replication_docs_rep_queue',
        'replication_meta_latency_aggr',
        'vb_active_num',
        'vb_active_queue_drain',
        'vb_active_queue_size',
        'vb_active_resident_items_ratio',
        'vb_avg_total_queue_age',
        'vb_pending_ops_create',
        'vb_pending_queue_fill',
        'vb_replica_curr_items',
        'vb_replica_meta_data_memory',
        'vb_replica_num',
        'vb_replica_queue_size',
        'xdc_ops',
    ])

    def _create_metrics(self, data, tags=None):
        storage_totals = data['stats']['storageTotals']
        for key, storage_type in storage_totals.items():
            for metric_name, val in storage_type.items():
                if val is not None:
                    metric_name = '.'.join(['couchbase', key, self.camel_case_to_joined_lower(metric_name)])
                    self.gauge(metric_name, val, tags=tags)

        for bucket_name, bucket_stats in data['buckets'].items():
            for metric_name, val in bucket_stats.items():
                if val is not None:
                    norm_metric_name = self.camel_case_to_joined_lower(metric_name)
                    if norm_metric_name in self.BUCKET_STATS:
                        full_metric_name = '.'.join(['couchbase', 'by_bucket', norm_metric_name])
                        metric_tags = list(tags)
                        metric_tags.append('bucket:%s' % bucket_name)
                        self.gauge(full_metric_name, val[0], tags=metric_tags, device_name=bucket_name)

        for node_name, node_stats in data['nodes'].items():
            for metric_name, val in node_stats['interestingStats'].items():
                if val is not None:
                    metric_name = '.'.join(['couchbase', 'by_node', self.camel_case_to_joined_lower(metric_name)])
                    metric_tags = list(tags)
                    metric_tags.append('node:%s' % node_name)
                    self.gauge(metric_name, val, tags=metric_tags, device_name=node_name)

    def _get_stats(self, url, instance):
        """ Hit a given URL and return the parsed json. """
        self.log.debug('Fetching Couchbase stats at url: %s' % url)

        timeout = float(instance.get('timeout', DEFAULT_TIMEOUT))

        auth = None
        if 'user' in instance and 'password' in instance:
            auth = (instance['user'], instance['password'])

        r = requests.get(url, auth=auth, headers=headers(self.agentConfig),
            timeout=timeout)
        r.raise_for_status()
        return r.json()

    def check(self, instance):
        server = instance.get('server', None)
        if server is None:
            raise Exception("The server must be specified")
        tags = instance.get('tags', [])
        # Clean up tags in case there was a None entry in the instance
        # e.g. if the yaml contains tags: but no actual tags
        if tags is None:
            tags = []
        else:
            tags = list(set(tags))
        tags.append('instance:%s' % server)
        data = self.get_data(server, instance)
        self._create_metrics(data, tags=list(set(tags)))

    def get_data(self, server, instance):
        # The dictionary to be returned.
        couchbase = {
            'stats': None,
            'buckets': {},
            'nodes': {}
        }

        # build couchbase stats entry point
        url = '%s%s' % (server, COUCHBASE_STATS_PATH)

        # Fetch initial stats and capture a service check based on response.
        service_check_tags = ['instance:%s' % server]
        try:
            overall_stats = self._get_stats(url, instance)
            # No overall stats? bail out now
            if overall_stats is None:
                raise Exception("No data returned from couchbase endpoint: %s" % url)
        except requests.exceptions.HTTPError as e:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                tags=service_check_tags, message=str(e.message))
            raise
        except Exception as e:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.CRITICAL,
                tags=service_check_tags, message=str(e))
            raise
        else:
            self.service_check(self.SERVICE_CHECK_NAME, AgentCheck.OK,
                tags=service_check_tags)

        couchbase['stats'] = overall_stats

        nodes = overall_stats['nodes']

        # Next, get all the nodes
        if nodes is not None:
            for node in nodes:
                couchbase['nodes'][node['hostname']] = node

        # Next, get all buckets .
        endpoint = overall_stats['buckets']['uri']

        url = '%s%s' % (server, endpoint)
        buckets = self._get_stats(url, instance)

        if buckets is not None:
            for bucket in buckets:
                bucket_name = bucket['name']

                # Fetch URI for the stats bucket
                endpoint = bucket['stats']['uri']
                url = '%s%s' % (server, endpoint)

                try:
                    bucket_stats = self._get_stats(url, instance)
                except requests.exceptions.HTTPError:
                    url_backup = '%s/pools/nodes/buckets/%s/stats' % (server, bucket_name)
                    bucket_stats = self._get_stats(url_backup, instance)

                bucket_samples = bucket_stats['op']['samples']
                if bucket_samples is not None:
                    couchbase['buckets'][bucket['name']] = bucket_samples

        return couchbase

    # Takes a camelCased variable and returns a joined_lower equivalent.
    # Returns input if non-camelCase variable is detected.
    def camel_case_to_joined_lower(self, variable):
        # replace non-word with _
        converted_variable = re.sub('\W+', '_', variable)

        # insert _ in front of capital letters and lowercase the string
        converted_variable = re.sub('([A-Z])', '_\g<1>', converted_variable).lower()

        # remove duplicate _
        converted_variable = re.sub('_+', '_', converted_variable)

        # handle special case of starting/ending underscores
        converted_variable = re.sub('^_|_$', '', converted_variable)

        return converted_variable
