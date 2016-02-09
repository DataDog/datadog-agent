# stdlib
import threading

# 3p
try:
    import psutil
except ImportError:
    psutil = None

# project
from checks import AgentCheck
from checks.metric_types import MetricTypes
from config import _is_affirmative

MAX_THREADS_COUNT = 50
MAX_COLLECTION_TIME = 30
MAX_EMIT_TIME = 5
MAX_CPU_PCT = 10


class UnsupportedMetricType(Exception):
    """
    Raised by :class:`AgentMetrics` when a metric type outside outside of AgentMetrics.ALLOWED_METRIC_TYPES
    is requested for measurement of a particular statistic
    """
    def __init__(self, metric_name, metric_type):
        message = 'Unsupported Metric Type for {0} : {1}'.format(metric_name, metric_type)
        Exception.__init__(self, message)

class AgentMetrics(AgentCheck):
    """
    New-style version of `CollectorMetrics`
    Gets information about agent performance on every collector loop
    """

    def __init__(self, *args, **kwargs):
        AgentCheck.__init__(self, *args, **kwargs)
        self._collector_payload = {}
        self._metric_context = {}

    def _psutil_config_to_stats(self, instance):
        """
        Reads `init_config` for `psutil` methods to call on the current process
        Calls those methods and stores the raw output

        :returns a dictionary of statistic_name: value
        """
        process_metrics = instance.get('process_metrics', self.init_config.get('process_metrics', None))
        if not process_metrics:
            self.log.error('No metrics configured for AgentMetrics check!')
            return {}

        methods, metric_types = zip(
            *[(p['name'], p.get('type', MetricTypes.GAUGE))
                for p in process_metrics if _is_affirmative(p.get('active'))]
        )

        names_to_metric_types = {}
        for i, m in enumerate(methods):
            names_to_metric_types[AgentCheck._get_statistic_name_from_method(m)] = metric_types[i]

        stats = AgentCheck._collect_internal_stats(methods)
        return stats, names_to_metric_types

    def _send_single_metric(self, metric_name, metric_value, metric_type):
        if metric_type == MetricTypes.GAUGE:
            self.gauge(metric_name, metric_value)
        elif metric_type == MetricTypes.RATE:
            self.rate(metric_name, metric_value)
        else:
            raise UnsupportedMetricType(metric_name, metric_type)

    def _register_psutil_metrics(self, stats, names_to_metric_types):
        """
        Saves sample metrics from psutil

        :param stats: a dictionary that looks like:
        {
         'memory_info': OrderedDict([('rss', 24395776), ('vms', 144666624)]),
         'io_counters': OrderedDict([('read_count', 4536),
                                    ('write_count', 100),
                                    ('read_bytes', 0),
                                    ('write_bytes', 61440)])
         ...
         }

         This creates a metric like `datadog.agent.collector.{key_1}.{key_2}` where key_1 is a top-level
         key in `stats`, and key_2 is a nested key.
         E.g. datadog.agent.collector.memory_info.rss
        """

        base_metric = 'datadog.agent.collector.{0}.{1}'
        # TODO: May have to call self.normalize(metric_name) to get a compliant name
        for k, v in stats.iteritems():
            metric_type = names_to_metric_types[k]
            if isinstance(v, dict):
                for _k, _v in v.iteritems():
                    full_metric_name = base_metric.format(k, _k)
                    self._send_single_metric(full_metric_name, _v, metric_type)
            else:
                full_metric_name = 'datadog.agent.collector.{0}'.format(k)
                self._send_single_metric(full_metric_name, v, metric_type)

    def set_metric_context(self, payload, context):
        self._collector_payload = payload
        self._metric_context = context

    def get_metric_context(self):
        return self._collector_payload, self._metric_context

    def check(self, instance):
        if self.in_developer_mode:
            stats, names_to_metric_types = self._psutil_config_to_stats(instance)
            self._register_psutil_metrics(stats, names_to_metric_types)

        payload, context = self.get_metric_context()
        collection_time = context.get('collection_time', None)
        emit_time = context.get('emit_time', None)
        cpu_time = context.get('cpu_time', None)

        if threading.activeCount() > MAX_THREADS_COUNT:
            self.gauge('datadog.agent.collector.threads.count', threading.activeCount())
            self.log.info("Thread count is high: %d" % threading.activeCount())

        collect_time_exceeds_threshold = collection_time > MAX_COLLECTION_TIME
        if collection_time is not None and \
                (collect_time_exceeds_threshold or self.in_developer_mode):

            self.gauge('datadog.agent.collector.collection.time', collection_time)
            if collect_time_exceeds_threshold:
                self.log.info("Collection time (s) is high: %.1f, metrics count: %d, events count: %d",
                              collection_time, len(payload['metrics']), len(payload['events']))

        emit_time_exceeds_threshold = emit_time > MAX_EMIT_TIME
        if emit_time is not None and \
                (emit_time_exceeds_threshold or self.in_developer_mode):
            self.gauge('datadog.agent.emitter.emit.time', emit_time)
            if emit_time_exceeds_threshold:
                self.log.info("Emit time (s) is high: %.1f, metrics count: %d, events count: %d",
                              emit_time, len(payload['metrics']), len(payload['events']))

        if cpu_time is not None:
            try:
                cpu_used_pct = 100.0 * float(cpu_time)/float(collection_time)
                if cpu_used_pct > MAX_CPU_PCT:
                    self.gauge('datadog.agent.collector.cpu.used', cpu_used_pct)
                    self.log.info("CPU consumed (%%) is high: %.1f, metrics count: %d, events count: %d",
                                  cpu_used_pct, len(payload['metrics']), len(payload['events']))
            except Exception, e:
                self.log.debug("Couldn't compute cpu used by collector with values %s %s %s",
                               cpu_time, collection_time, str(e))
