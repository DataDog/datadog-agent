# stdlib
from collections import defaultdict, namedtuple
import time
import urlparse

# 3p
import requests

# project
from checks import AgentCheck
from config import _is_affirmative
from util import headers


class NodeNotFound(Exception):
    pass


ESInstanceConfig = namedtuple(
    'ESInstanceConfig', [
        'pshard_stats',
        'cluster_stats',
        'password',
        'service_check_tags',
        'tags',
        'timeout',
        'url',
        'username',
    ])


class ESCheck(AgentCheck):
    SERVICE_CHECK_CONNECT_NAME = 'elasticsearch.can_connect'
    SERVICE_CHECK_CLUSTER_STATUS = 'elasticsearch.cluster_health'

    DEFAULT_TIMEOUT = 5

    # Clusterwise metrics, pre aggregated on ES, compatible with all ES versions
    PRIMARY_SHARD_METRICS = {
        "elasticsearch.primaries.docs.count": ("gauge", "_all.primaries.docs.count"),
        "elasticsearch.primaries.docs.deleted": ("gauge", "_all.primaries.docs.deleted"),
        "elasticsearch.primaries.store.size": ("gauge", "_all.primaries.store.size_in_bytes"),
        "elasticsearch.primaries.indexing.index.total": ("gauge", "_all.primaries.indexing.index_total"),
        "elasticsearch.primaries.indexing.index.time": ("gauge", "_all.primaries.indexing.index_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.primaries.indexing.index.current": ("gauge", "_all.primaries.indexing.index_current"),
        "elasticsearch.primaries.indexing.delete.total": ("gauge", "_all.primaries.indexing.delete_total"),
        "elasticsearch.primaries.indexing.delete.time": ("gauge", "_all.primaries.indexing.delete_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.primaries.indexing.delete.current": ("gauge", "_all.primaries.indexing.delete_current"),
        "elasticsearch.primaries.get.total": ("gauge", "_all.primaries.get.total"),
        "elasticsearch.primaries.get.time": ("gauge", "_all.primaries.get.time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.primaries.get.current": ("gauge", "_all.primaries.get.current"),
        "elasticsearch.primaries.get.exists.total": ("gauge", "_all.primaries.get.exists_total"),
        "elasticsearch.primaries.get.exists.time": ("gauge", "_all.primaries.get.exists_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.primaries.get.missing.total": ("gauge", "_all.primaries.get.missing_total"),
        "elasticsearch.primaries.get.missing.time": ("gauge", "_all.primaries.get.missing_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.primaries.search.query.total": ("gauge", "_all.primaries.search.query_total"),
        "elasticsearch.primaries.search.query.time": ("gauge", "_all.primaries.search.query_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.primaries.search.query.current": ("gauge", "_all.primaries.search.query_current"),
        "elasticsearch.primaries.search.fetch.total": ("gauge", "_all.primaries.search.fetch_total"),
        "elasticsearch.primaries.search.fetch.time": ("gauge", "_all.primaries.search.fetch_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.primaries.search.fetch.current": ("gauge", "_all.primaries.search.fetch_current")
    }

    PRIMARY_SHARD_METRICS_POST_1_0 = {
        "elasticsearch.primaries.merges.current": ("gauge", "_all.primaries.merges.current"),
        "elasticsearch.primaries.merges.current.docs": ("gauge", "_all.primaries.merges.current_docs"),
        "elasticsearch.primaries.merges.current.size": ("gauge", "_all.primaries.merges.current_size_in_bytes"),
        "elasticsearch.primaries.merges.total": ("gauge", "_all.primaries.merges.total"),
        "elasticsearch.primaries.merges.total.time": ("gauge", "_all.primaries.merges.total_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.primaries.merges.total.docs": ("gauge", "_all.primaries.merges.total_docs"),
        "elasticsearch.primaries.merges.total.size": ("gauge", "_all.primaries.merges.total_size_in_bytes"),
        "elasticsearch.primaries.refresh.total": ("gauge", "_all.primaries.refresh.total"),
        "elasticsearch.primaries.refresh.total.time": ("gauge", "_all.primaries.refresh.total_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.primaries.flush.total": ("gauge", "_all.primaries.flush.total"),
        "elasticsearch.primaries.flush.total.time": ("gauge", "_all.primaries.flush.total_time_in_millis", lambda v: float(v)/1000)
    }

    STATS_METRICS = {  # Metrics that are common to all Elasticsearch versions
        "elasticsearch.docs.count": ("gauge", "indices.docs.count"),
        "elasticsearch.docs.deleted": ("gauge", "indices.docs.deleted"),
        "elasticsearch.store.size": ("gauge", "indices.store.size_in_bytes"),
        "elasticsearch.indexing.index.total": ("gauge", "indices.indexing.index_total"),
        "elasticsearch.indexing.index.time": ("gauge", "indices.indexing.index_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.indexing.index.current": ("gauge", "indices.indexing.index_current"),
        "elasticsearch.indexing.delete.total": ("gauge", "indices.indexing.delete_total"),
        "elasticsearch.indexing.delete.time": ("gauge", "indices.indexing.delete_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.indexing.delete.current": ("gauge", "indices.indexing.delete_current"),
        "elasticsearch.get.total": ("gauge", "indices.get.total"),
        "elasticsearch.get.time": ("gauge", "indices.get.time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.get.current": ("gauge", "indices.get.current"),
        "elasticsearch.get.exists.total": ("gauge", "indices.get.exists_total"),
        "elasticsearch.get.exists.time": ("gauge", "indices.get.exists_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.get.missing.total": ("gauge", "indices.get.missing_total"),
        "elasticsearch.get.missing.time": ("gauge", "indices.get.missing_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.search.query.total": ("gauge", "indices.search.query_total"),
        "elasticsearch.search.query.time": ("gauge", "indices.search.query_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.search.query.current": ("gauge", "indices.search.query_current"),
        "elasticsearch.search.fetch.total": ("gauge", "indices.search.fetch_total"),
        "elasticsearch.search.fetch.time": ("gauge", "indices.search.fetch_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.search.fetch.current": ("gauge", "indices.search.fetch_current"),
        "elasticsearch.merges.current": ("gauge", "indices.merges.current"),
        "elasticsearch.merges.current.docs": ("gauge", "indices.merges.current_docs"),
        "elasticsearch.merges.current.size": ("gauge", "indices.merges.current_size_in_bytes"),
        "elasticsearch.merges.total": ("gauge", "indices.merges.total"),
        "elasticsearch.merges.total.time": ("gauge", "indices.merges.total_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.merges.total.docs": ("gauge", "indices.merges.total_docs"),
        "elasticsearch.merges.total.size": ("gauge", "indices.merges.total_size_in_bytes"),
        "elasticsearch.refresh.total": ("gauge", "indices.refresh.total"),
        "elasticsearch.refresh.total.time": ("gauge", "indices.refresh.total_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.flush.total": ("gauge", "indices.flush.total"),
        "elasticsearch.flush.total.time": ("gauge", "indices.flush.total_time_in_millis", lambda v: float(v)/1000),
        "elasticsearch.process.open_fd": ("gauge", "process.open_file_descriptors"),
        "elasticsearch.transport.rx_count": ("gauge", "transport.rx_count"),
        "elasticsearch.transport.tx_count": ("gauge", "transport.tx_count"),
        "elasticsearch.transport.rx_size": ("gauge", "transport.rx_size_in_bytes"),
        "elasticsearch.transport.tx_size": ("gauge", "transport.tx_size_in_bytes"),
        "elasticsearch.transport.server_open": ("gauge", "transport.server_open"),
        "elasticsearch.thread_pool.bulk.active": ("gauge", "thread_pool.bulk.active"),
        "elasticsearch.thread_pool.bulk.threads": ("gauge", "thread_pool.bulk.threads"),
        "elasticsearch.thread_pool.bulk.queue": ("gauge", "thread_pool.bulk.queue"),
        "elasticsearch.thread_pool.flush.active": ("gauge", "thread_pool.flush.active"),
        "elasticsearch.thread_pool.flush.threads": ("gauge", "thread_pool.flush.threads"),
        "elasticsearch.thread_pool.flush.queue": ("gauge", "thread_pool.flush.queue"),
        "elasticsearch.thread_pool.generic.active": ("gauge", "thread_pool.generic.active"),
        "elasticsearch.thread_pool.generic.threads": ("gauge", "thread_pool.generic.threads"),
        "elasticsearch.thread_pool.generic.queue": ("gauge", "thread_pool.generic.queue"),
        "elasticsearch.thread_pool.get.active": ("gauge", "thread_pool.get.active"),
        "elasticsearch.thread_pool.get.threads": ("gauge", "thread_pool.get.threads"),
        "elasticsearch.thread_pool.get.queue": ("gauge", "thread_pool.get.queue"),
        "elasticsearch.thread_pool.index.active": ("gauge", "thread_pool.index.active"),
        "elasticsearch.thread_pool.index.threads": ("gauge", "thread_pool.index.threads"),
        "elasticsearch.thread_pool.index.queue": ("gauge", "thread_pool.index.queue"),
        "elasticsearch.thread_pool.management.active": ("gauge", "thread_pool.management.active"),
        "elasticsearch.thread_pool.management.threads": ("gauge", "thread_pool.management.threads"),
        "elasticsearch.thread_pool.management.queue": ("gauge", "thread_pool.management.queue"),
        "elasticsearch.thread_pool.merge.active": ("gauge", "thread_pool.merge.active"),
        "elasticsearch.thread_pool.merge.threads": ("gauge", "thread_pool.merge.threads"),
        "elasticsearch.thread_pool.merge.queue": ("gauge", "thread_pool.merge.queue"),
        "elasticsearch.thread_pool.percolate.active": ("gauge", "thread_pool.percolate.active"),
        "elasticsearch.thread_pool.percolate.threads": ("gauge", "thread_pool.percolate.threads"),
        "elasticsearch.thread_pool.percolate.queue": ("gauge", "thread_pool.percolate.queue"),
        "elasticsearch.thread_pool.refresh.active": ("gauge", "thread_pool.refresh.active"),
        "elasticsearch.thread_pool.refresh.threads": ("gauge", "thread_pool.refresh.threads"),
        "elasticsearch.thread_pool.refresh.queue": ("gauge", "thread_pool.refresh.queue"),
        "elasticsearch.thread_pool.search.active": ("gauge", "thread_pool.search.active"),
        "elasticsearch.thread_pool.search.threads": ("gauge", "thread_pool.search.threads"),
        "elasticsearch.thread_pool.search.queue": ("gauge", "thread_pool.search.queue"),
        "elasticsearch.thread_pool.snapshot.active": ("gauge", "thread_pool.snapshot.active"),
        "elasticsearch.thread_pool.snapshot.threads": ("gauge", "thread_pool.snapshot.threads"),
        "elasticsearch.thread_pool.snapshot.queue": ("gauge", "thread_pool.snapshot.queue"),
        "elasticsearch.http.current_open": ("gauge", "http.current_open"),
        "elasticsearch.http.total_opened": ("gauge", "http.total_opened"),
        "jvm.mem.heap_committed": ("gauge", "jvm.mem.heap_committed_in_bytes"),
        "jvm.mem.heap_used": ("gauge", "jvm.mem.heap_used_in_bytes"),
        "jvm.mem.heap_in_use": ("gauge", "jvm.mem.heap_used_percent"),
        "jvm.mem.heap_max": ("gauge", "jvm.mem.heap_max_in_bytes"),
        "jvm.mem.non_heap_committed": ("gauge", "jvm.mem.non_heap_committed_in_bytes"),
        "jvm.mem.non_heap_used": ("gauge", "jvm.mem.non_heap_used_in_bytes"),
        "jvm.threads.count": ("gauge", "jvm.threads.count"),
        "jvm.threads.peak_count": ("gauge", "jvm.threads.peak_count"),
    }

    JVM_METRICS_POST_0_90_10 = {
        "jvm.gc.collectors.young.count": ("gauge", "jvm.gc.collectors.young.collection_count"),
        "jvm.gc.collectors.young.collection_time": ("gauge", "jvm.gc.collectors.young.collection_time_in_millis", lambda v: float(v)/1000),
        "jvm.gc.collectors.old.count": ("gauge", "jvm.gc.collectors.old.collection_count"),
        "jvm.gc.collectors.old.collection_time": ("gauge", "jvm.gc.collectors.old.collection_time_in_millis", lambda v: float(v)/1000)
    }

    JVM_METRICS_PRE_0_90_10 = {
        "jvm.gc.concurrent_mark_sweep.count": ("gauge", "jvm.gc.collectors.ConcurrentMarkSweep.collection_count"),
        "jvm.gc.concurrent_mark_sweep.collection_time": ("gauge", "jvm.gc.collectors.ConcurrentMarkSweep.collection_time_in_millis", lambda v: float(v)/1000),
        "jvm.gc.par_new.count": ("gauge", "jvm.gc.collectors.ParNew.collection_count"),
        "jvm.gc.par_new.collection_time": ("gauge", "jvm.gc.collectors.ParNew.collection_time_in_millis", lambda v: float(v)/1000),
        "jvm.gc.collection_count": ("gauge", "jvm.gc.collection_count"),
        "jvm.gc.collection_time": ("gauge", "jvm.gc.collection_time_in_millis", lambda v: float(v)/1000),
    }

    ADDITIONAL_METRICS_POST_0_90_5 = {
        "elasticsearch.search.fetch.open_contexts": ("gauge", "indices.search.open_contexts"),
        "elasticsearch.cache.filter.evictions": ("gauge", "indices.filter_cache.evictions"),
        "elasticsearch.cache.filter.size": ("gauge", "indices.filter_cache.memory_size_in_bytes"),
        "elasticsearch.id_cache.size": ("gauge", "indices.id_cache.memory_size_in_bytes"),
        "elasticsearch.fielddata.size": ("gauge", "indices.fielddata.memory_size_in_bytes"),
        "elasticsearch.fielddata.evictions": ("gauge", "indices.fielddata.evictions"),
    }

    ADDITIONAL_METRICS_PRE_0_90_5 = {
        "elasticsearch.cache.field.evictions": ("gauge", "indices.cache.field_evictions"),
        "elasticsearch.cache.field.size": ("gauge", "indices.cache.field_size_in_bytes"),
        "elasticsearch.cache.filter.count": ("gauge", "indices.cache.filter_count"),
        "elasticsearch.cache.filter.evictions": ("gauge", "indices.cache.filter_evictions"),
        "elasticsearch.cache.filter.size": ("gauge", "indices.cache.filter_size_in_bytes"),
    }

    CLUSTER_HEALTH_METRICS = {
        "elasticsearch.number_of_nodes": ("gauge", "number_of_nodes"),
        "elasticsearch.number_of_data_nodes": ("gauge", "number_of_data_nodes"),
        "elasticsearch.active_primary_shards": ("gauge", "active_primary_shards"),
        "elasticsearch.active_shards": ("gauge", "active_shards"),
        "elasticsearch.relocating_shards": ("gauge", "relocating_shards"),
        "elasticsearch.initializing_shards": ("gauge", "initializing_shards"),
        "elasticsearch.unassigned_shards": ("gauge", "unassigned_shards"),
        "elasticsearch.cluster_status": ("gauge", "status", lambda v: {"red": 0, "yellow": 1, "green": 2}.get(v, -1)),
    }

    CLUSTER_PENDING_TASKS = {
        "elasticsearch.pending_tasks_total": ("gauge", "pending_task_total"),
        "elasticsearch.pending_tasks_priority_high": ("gauge", "pending_tasks_priority_high"),
        "elasticsearch.pending_tasks_priority_urgent": ("gauge", "pending_tasks_priority_urgent")
    }

    SOURCE_TYPE_NAME = 'elasticsearch'

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)

        # Host status needs to persist across all checks
        self.cluster_status = {}

    def get_instance_config(self, instance):
        url = instance.get('url')
        if url is None:
            raise Exception("An url must be specified in the instance")

        pshard_stats = _is_affirmative(instance.get('pshard_stats', False))

        cluster_stats = _is_affirmative(instance.get('cluster_stats', False))
        if 'is_external' in instance:
            cluster_stats = _is_affirmative(instance.get('is_external', False))

        # Support URLs that have a path in them from the config, for
        # backwards-compatibility.
        parsed = urlparse.urlparse(url)
        if parsed[2] != "":
            url = "%s://%s" % (parsed[0], parsed[1])
        port = parsed.port
        host = parsed.hostname
        service_check_tags = [
            'host:%s' % host,
            'port:%s' % port
        ]

        # Tag by URL so we can differentiate the metrics
        # from multiple instances
        tags = ['url:%s' % url]
        tags.extend(instance.get('tags', []))

        timeout = instance.get('timeout') or self.DEFAULT_TIMEOUT

        config = ESInstanceConfig(
            pshard_stats=pshard_stats,
            cluster_stats=cluster_stats,
            password=instance.get('password'),
            service_check_tags=service_check_tags,
            tags=tags,
            timeout=timeout,
            url=url,
            username=instance.get('username')
        )
        return config

    def check(self, instance):
        config = self.get_instance_config(instance)

        # Check ES version for this instance and define parameters
        # (URLs and metrics) accordingly
        version = self._get_es_version(config)

        health_url, nodes_url, stats_url, pshard_stats_url, pending_tasks_url, stats_metrics, \
            pshard_stats_metrics = self._define_params(version, config.cluster_stats)

        # Load clusterwise data
        if config.pshard_stats:
            pshard_stats_url = urlparse.urljoin(config.url, pshard_stats_url)
            pshard_stats_data = self._get_data(pshard_stats_url, config)
            self._process_pshard_stats_data(pshard_stats_data, config, pshard_stats_metrics)

        # Load stats data.
        stats_url = urlparse.urljoin(config.url, stats_url)
        stats_data = self._get_data(stats_url, config)
        self._process_stats_data(nodes_url, stats_data, stats_metrics, config)

        # Load the health data.
        health_url = urlparse.urljoin(config.url, health_url)
        health_data = self._get_data(health_url, config)
        self._process_health_data(health_data, config)

        # Load the pending_tasks data.
        pending_tasks_url = urlparse.urljoin(config.url, pending_tasks_url)
        pending_tasks_data = self._get_data(pending_tasks_url, config)
        self._process_pending_tasks_data(pending_tasks_data, config)

        # If we're here we did not have any ES conn issues
        self.service_check(
            self.SERVICE_CHECK_CONNECT_NAME,
            AgentCheck.OK,
            tags=config.service_check_tags
        )

    def _get_es_version(self, config):
        """ Get the running version of elasticsearch.
        """
        try:
            data = self._get_data(config.url, config, send_sc=False)
            version = map(int, data['version']['number'].split('.')[0:3])
        except Exception, e:
            self.warning(
                "Error while trying to get Elasticsearch version "
                "from %s %s"
                % (config.url, str(e))
            )
            version = [1, 0, 0]

        self.service_metadata('version', version)
        self.log.debug("Elasticsearch version is %s" % version)
        return version

    def _define_params(self, version, cluster_stats):
        """ Define the set of URLs and METRICS to use depending on the
            running ES version.
        """

        pshard_stats_url = "/_stats"

        if version >= [0, 90, 10]:
            # ES versions 0.90.10 and above
            health_url = "/_cluster/health?pretty=true"
            nodes_url = "/_nodes?network=true"
            pending_tasks_url = "/_cluster/pending_tasks?pretty=true"

            # For "external" clusters, we want to collect from all nodes.
            if cluster_stats:
                stats_url = "/_nodes/stats?all=true"
            else:
                stats_url = "/_nodes/_local/stats?all=true"

            additional_metrics = self.JVM_METRICS_POST_0_90_10
        else:
            health_url = "/_cluster/health?pretty=true"
            nodes_url = "/_cluster/nodes?network=true"
            pending_tasks_url = None
            if cluster_stats:
                stats_url = "/_cluster/nodes/stats?all=true"
            else:
                stats_url = "/_cluster/nodes/_local/stats?all=true"

            additional_metrics = self.JVM_METRICS_PRE_0_90_10

        stats_metrics = dict(self.STATS_METRICS)
        stats_metrics.update(additional_metrics)

        if version >= [0, 90, 5]:
            # ES versions 0.90.5 and above
            additional_metrics = self.ADDITIONAL_METRICS_POST_0_90_5
        else:
            # ES version 0.90.4 and below
            additional_metrics = self.ADDITIONAL_METRICS_PRE_0_90_5

        stats_metrics.update(additional_metrics)

        # Version specific stats metrics about the primary shards
        pshard_stats_metrics = dict(self.PRIMARY_SHARD_METRICS)

        if version >= [1, 0, 0]:
            additional_metrics = self.PRIMARY_SHARD_METRICS_POST_1_0

        pshard_stats_metrics.update(additional_metrics)

        return health_url, nodes_url, stats_url, pshard_stats_url, pending_tasks_url, \
            stats_metrics, pshard_stats_metrics

    def _get_data(self, url, config, send_sc=True):
        """ Hit a given URL and return the parsed json
        """
        # Load basic authentication configuration, if available.
        if config.username and config.password:
            auth = (config.username, config.password)
        else:
            auth = None

        try:
            resp = requests.get(
                url,
                timeout=config.timeout,
                headers=headers(self.agentConfig),
                auth=auth
            )
            resp.raise_for_status()
        except Exception as e:
            if send_sc:
                self.service_check(
                    self.SERVICE_CHECK_CONNECT_NAME,
                    AgentCheck.CRITICAL,
                    message="Error {0} when hitting {1}".format(e, url),
                    tags=config.service_check_tags
                )
            raise

        return resp.json()

    def _process_pending_tasks_data(self, data, config):
        p_tasks = defaultdict(int)

        for task in data.get('tasks', []):
            p_tasks[task.get('priority')] += 1

        node_data = {
            'pending_task_total':               sum(p_tasks.values()),
            'pending_tasks_priority_high':      p_tasks['high'],
            'pending_tasks_priority_urgent':    p_tasks['urgent'],
        }

        for metric in self.CLUSTER_PENDING_TASKS:
            # metric description
            desc = self.CLUSTER_PENDING_TASKS[metric]
            self._process_metric(node_data, metric, *desc, tags=config.tags)

    def _process_stats_data(self, nodes_url, data, stats_metrics, config):
        cluster_stats = config.cluster_stats
        for node_name in data['nodes']:
            node_data = data['nodes'][node_name]
            # On newer version of ES it's "host" not "hostname"
            node_hostname = node_data.get(
                'hostname', node_data.get('host', None))

            # Override the metric hostname if we're hitting an external cluster
            metric_hostname = node_hostname if cluster_stats else None

            for metric, desc in stats_metrics.iteritems():
                self._process_metric(
                    node_data, metric, *desc, tags=config.tags,
                    hostname=metric_hostname)

    def _process_pshard_stats_data(self, data, config, pshard_stats_metrics):
        for metric, desc in pshard_stats_metrics.iteritems():
            self._process_metric(data, metric, *desc, tags=config.tags)

    def _process_metric(self, data, metric, xtype, path, xform=None,
                        tags=None, hostname=None):
        """data: dictionary containing all the stats
        metric: datadog metric
        path: corresponding path in data, flattened, e.g. thread_pool.bulk.queue
        xfom: a lambda to apply to the numerical value
        """
        value = data

        # Traverse the nested dictionaries
        for key in path.split('.'):
            if value is not None:
                value = value.get(key, None)
            else:
                break

        if value is not None:
            if xform:
                value = xform(value)
            if xtype == "gauge":
                self.gauge(metric, value, tags=tags, hostname=hostname)
            else:
                self.rate(metric, value, tags=tags, hostname=hostname)
        else:
            self._metric_not_found(metric, path)

    def _process_health_data(self, data, config):
        if self.cluster_status.get(config.url) is None:
            self.cluster_status[config.url] = data['status']
            if data['status'] in ["yellow", "red"]:
                event = self._create_event(data['status'], tags=config.tags)
                self.event(event)

        if data['status'] != self.cluster_status.get(config.url):
            self.cluster_status[config.url] = data['status']
            event = self._create_event(data['status'], tags=config.tags)
            self.event(event)

        for metric, desc in self.CLUSTER_HEALTH_METRICS.iteritems():
            self._process_metric(data, metric, *desc, tags=config.tags)

        # Process the service check
        cluster_status = data['status']
        if cluster_status == 'green':
            status = AgentCheck.OK
            data['tag'] = "OK"
        elif cluster_status == 'yellow':
            status = AgentCheck.WARNING
            data['tag'] = "WARN"
        else:
            status = AgentCheck.CRITICAL
            data['tag'] = "ALERT"

        msg = "{tag} on cluster \"{cluster_name}\" "\
              "| active_shards={active_shards} "\
              "| initializing_shards={initializing_shards} "\
              "| relocating_shards={relocating_shards} "\
              "| unassigned_shards={unassigned_shards} "\
              "| timed_out={timed_out}" \
              .format(**data)

        self.service_check(
            self.SERVICE_CHECK_CLUSTER_STATUS,
            status,
            message=msg,
            tags=config.service_check_tags
        )

    def _metric_not_found(self, metric, path):
        self.log.debug("Metric not found: %s -> %s", path, metric)

    def _create_event(self, status, tags=None):
        hostname = self.hostname.decode('utf-8')
        if status == "red":
            alert_type = "error"
            msg_title = "%s is %s" % (hostname, status)

        elif status == "yellow":
            alert_type = "warning"
            msg_title = "%s is %s" % (hostname, status)

        else:
            # then it should be green
            alert_type = "success"
            msg_title = "%s recovered as %s" % (hostname, status)

        msg = "ElasticSearch: %s just reported as %s" % (hostname, status)

        return {
            'timestamp': int(time.time()),
            'event_type': 'elasticsearch',
            'host': hostname,
            'msg_text': msg,
            'msg_title': msg_title,
            'alert_type': alert_type,
            'source_type_name': "elasticsearch",
            'event_object': hostname,
            'tags': tags
        }
