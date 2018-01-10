# (C) Datadog, Inc. 2018
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

import re
import requests
from google.protobuf.internal.decoder import _DecodeVarint32  # pylint: disable=E0611,E0401
from checks import AgentCheck
from . import metrics_pb2

# Prometheus check is a mother class providing a structure and some helpers
# to collect metrics, events and service checks exposed via Prometheus.
#
# It must be noted that if the check implementing this class is not officially
# supported
# its metrics will count as cutom metrics and WILL impact billing.
#
# Minimal config for checks based on this class include:
#   - implementing the check method
#   - overriding self.NAMESPACE
#   - overriding self.metrics_mapper
#     AND/OR
#   - create method named after the prometheus metric they will handle (see self.prometheus_metric_name)
#

# Used to specify if you want to use the protobuf format or the text format when
# querying prometheus metrics
class PrometheusFormat:
    PROTOBUF = "PROTOBUF"
    TEXT = "TEXT"

class PrometheusCheck(AgentCheck):
    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        # message.type is the index in this array
        # see: https://github.com/prometheus/client_model/blob/master/ruby/lib/prometheus/client/model/metrics.pb.rb
        self.METRIC_TYPES = ['counter', 'gauge', 'summary', 'untyped', 'histogram']

        # patterns used for metrics and labels extraction form the prometheus
        # text format. Do not overwrite those
        self.metrics_pattern = re.compile(r'^(\w+)(.*)\s+([0-9.+eE,]+)$')
        self.lbl_pattern = re.compile(r'(\w+)="(.*?)"')

        # `NAMESPACE` is the prefix metrics will have. Need to be hardcoded in the
        # child check class.
        self.NAMESPACE = ''

        # `metrics_mapper` is a dictionnary where the keys are the metrics to capture
        # and the values are the corresponding metrics names to have in datadog.
        # Note: it is empty in the mother class but will need to be
        # overloaded/hardcoded in the final check not to be counted as custom metric.
        self.metrics_mapper = {}

        # Some metrics are ignored because they are duplicates or introduce a
        # very high cardinality. Metrics included in this list will be silently
        # skipped without a 'Unable to handle metric' debug line in the logs
        self.ignore_metrics = []

        # If the `labels_mapper` dictionnary is provided, the metrics labels names
        # in the `labels_mapper` will use the corresponding value as tag name
        # when sending the gauges.
        self.labels_mapper = {}

        # `exclude_labels` is an array of labels names to exclude. Those labels
        # will just not be added as tags when submitting the metric.
        self.exclude_labels = []

        # `type_overrides` is a dictionnary where the keys are prometheus metric names
        # and the values are a metric type (name as string) to use instead of the one
        # listed in the payload. It can be used to force a type on untyped metrics.
        # Note: it is empty in the mother class but will need to be
        # overloaded/hardcoded in the final check not to be counted as custom metric.
        self.type_overrides = {}

    def check(self, instance):
        """
        check should take care of getting the url and other params
        from the instance and using the utils to process messages and submit metrics.
        """
        raise NotImplementedError()

    def prometheus_metric_name(self, message, **kwargs):
        """ Example method"""
        pass

    class UnknownFormatError(Exception):
        def __init__(self, arg):
            self.args = arg

    def parse_metric_family(self, buf, content_type):
        """
        Gets the output data from a prometheus endpoint response along with its
        Content-type header and parses it into Prometheus classes (see [0])

        Parse the binary buffer in input, searching for Prometheus messages
        of type MetricFamily [0] delimited by a varint32 [1] when the
        content-type is a `application/vnd.google.protobuf`.

        [0] https://github.com/prometheus/client_model/blob/086fe7ca28bde6cec2acd5223423c1475a362858/metrics.proto#L76-%20%20L81
        [1] https://developers.google.com/protocol-buffers/docs/reference/java/com/google/protobuf/AbstractMessageLite#writeDelimitedTo(java.io.OutputStream)
        """
        if 'application/vnd.google.protobuf' in content_type:
            n = 0
            while n < len(buf):
                msg_len, new_pos = _DecodeVarint32(buf, n)
                n = new_pos
                msg_buf = buf[n:n+msg_len]
                n += msg_len

                message = metrics_pb2.MetricFamily()
                message.ParseFromString(msg_buf)

                # Lookup type overrides:
                if self.type_overrides and message.name in self.type_overrides:
                    new_type = self.type_overrides[message.name]
                    if new_type in self.METRIC_TYPES:
                        message.type = self.METRIC_TYPES.index(new_type)
                    else:
                        self.log.debug("type override %s for %s is not a valid type name" % (new_type, message.name))
                yield message
        elif 'text/plain' in content_type:
            messages = {}  # map with the name of the element (before the labels) and the list of occurrences with labels and values
            obj_map = {}   # map of the types of each metrics
            obj_help = {}  # help for the metrics
            for line in buf.splitlines():
                self._extract_metrics_from_string(line, messages, obj_map, obj_help)

            # Add type overrides:
            for m_name, m_type in self.type_overrides.iteritems():
                if m_type in self.METRIC_TYPES:
                    obj_map[m_name] = m_type
                else:
                    self.log.debug("type override %s for %s is not a valid type name" % (m_type,m_name))


            for _m in obj_map:
                if _m in messages or (obj_map[_m] == 'histogram' and '{}_bucket'.format(_m) in messages):
                    yield self._extract_metric_from_map(_m, messages, obj_map, obj_help)
        else:
            raise self.UnknownFormatError('Unsupported content-type provided: {}'.format(content_type))

    def _extract_metric_from_map(self, _m, messages, obj_map, obj_help):
        """
        Extracts MetricFamily objects from the maps generated by parsing the
        strings in _extract_metrics_from_string
        """
        _obj = metrics_pb2.MetricFamily()
        _obj.name = _m
        _obj.type = self.METRIC_TYPES.index(obj_map[_m])
        if _m in obj_help:
            _obj.help = obj_help[_m]
        # trick for histograms
        _newlbl = _m
        if obj_map[_m] == 'histogram':
            _newlbl = '{}_bucket'.format(_m)
        # Loop through the array of metrics ({labels, value}) built earlier
        for _metric in messages[_newlbl]:
            # in the case of quantiles and buckets, they need to be grouped by labels
            if obj_map[_m] in ['summary', 'histogram'] and len(_obj.metric) > 0:
                _label_exists = False
                _metric_minus = {k:v for k,v in _metric['labels'].items() if k not in ['quantile', 'le']}
                _metric_idx = 0
                for mls in _obj.metric:
                    _tmp_lbl = {idx.name:idx.value for idx in mls.label}
                    if _metric_minus == _tmp_lbl:
                        _label_exists = True
                        break
                    _metric_idx = _metric_idx + 1
                if _label_exists:
                    _g = _obj.metric[_metric_idx]
                else:
                    _g = _obj.metric.add()
            else:
                _g = _obj.metric.add()
            if obj_map[_m] == 'counter':
                _g.counter.value = float(_metric['value'])
            elif obj_map[_m] == 'gauge':
                _g.gauge.value = float(_metric['value'])
            elif obj_map[_m] == 'summary':
                if '{}_count'.format(_m) in messages:
                    _g.summary.sample_count = long(float(messages['{}_count'.format(_m)][0]['value']))
                if '{}_sum'.format(_m) in messages:
                    _g.summary.sample_sum = float(messages['{}_sum'.format(_m)][0]['value'])
            # TODO: see what can be done with the untyped metrics
            elif obj_map[_m] == 'histogram':
                if '{}_count'.format(_m) in messages:
                    _g.histogram.sample_count = long(float(messages['{}_count'.format(_m)][0]['value']))
                if '{}_sum'.format(_m) in messages:
                    _g.histogram.sample_sum = float(messages['{}_sum'.format(_m)][0]['value'])
            # last_metric = len(_obj.metric) - 1
            # if last_metric >= 0:
            for lbl in _metric['labels']:
                # In the string format, the quantiles are in the lablels
                if lbl == 'quantile':
                    # _q = _obj.metric[last_metric].summary.quantile.add()
                    _q = _g.summary.quantile.add()
                    _q.quantile = float(_metric['labels'][lbl])
                    _q.value = float(_metric['value'])
                # The upper_bounds are stored as "le" labels on string format
                elif obj_map[_m] == 'histogram' and lbl == 'le':
                    # _q = _obj.metric[last_metric].histogram.bucket.add()
                    _q = _g.histogram.bucket.add()
                    _q.upper_bound = float(_metric['labels'][lbl])
                    _q.cumulative_count = long(float(_metric['value']))
                else:
                    # labels deduplication
                    is_in_labels = False
                    for _existing_lbl in _g.label:
                        if lbl == _existing_lbl.name:
                            is_in_labels = True
                    if not is_in_labels:
                        _l = _g.label.add()
                        _l.name = lbl
                        _l.value = _metric['labels'][lbl]
        return _obj

    def _extract_metrics_from_string(self, line, messages, obj_map, obj_help):
        """
        Extracts the metrics from a line of metric and update the given
        dictionnaries (we take advantage of the reference of the dictionary here)
        """
        if line.startswith('# TYPE'):
            metric = line.split(' ')
            if len(metric) == 4:
                obj_map[metric[2]] = metric[3]  # line = # TYPE metric_name metric_type
        elif line.startswith('# HELP'):
            _h = line.split(' ', 3)
            if len(_h) == 4:
                obj_help[_h[2]] = _h[3]  # line = # HELP metric_name Help message...
        elif not line.startswith('#'):
            _match = self.metrics_pattern.match(line)
            if _match is not None:
                _g = _match.groups()
                _msg = []
                _lbls = self._extract_labels_from_string(_g[1])
                if _g[0] in messages:
                    _msg = messages[_g[0]]
                _msg.append({'labels': _lbls, 'value': _g[2]})
                messages[_g[0]] = _msg

    def _extract_labels_from_string(self,labels):
        """
        Extracts the labels from a string that looks like:
        {label_name_1="value 1", label_name_2="value 2"}
        """
        lbls = {}
        labels = labels.lstrip('{').rstrip('}')
        _lbls = self.lbl_pattern.findall(labels)
        for _lbl in _lbls:
            lbls[_lbl[0]] = _lbl[1]
        return lbls

    def process(self, endpoint, send_histograms_buckets=True, instance=None):
        """
        Polls the data from prometheus and pushes them as gauges
        `endpoint` is the metrics endpoint to use to poll metrics from Prometheus

        Note that if the instance has a 'tags' attribute, it will be pushed
        automatically as additionnal custom tags and added to the metrics
        """
        content_type, data = self.poll(endpoint)
        tags = []
        if instance is not None:
            tags = instance.get('tags', [])
        for metric in self.parse_metric_family(data, content_type):
            self.process_metric(metric, send_histograms_buckets=send_histograms_buckets, custom_tags=tags, instance=instance)

    def process_metric(self, message, send_histograms_buckets=True, custom_tags=None, **kwargs):
        """
        Handle a prometheus metric message according to the following flow:
            - search self.metrics_mapper for a prometheus.metric <--> datadog.metric mapping
            - call check method with the same name as the metric
            - log some info if none of the above worked

        `send_histograms_buckets` is used to specify if yes or no you want to send the buckets as tagged values when dealing with histograms.
        """
        try:
            if message.name in self.ignore_metrics:
                return  # Ignore the metric
            if message.name in self.metrics_mapper:
                self._submit(self.metrics_mapper[message.name], message, send_histograms_buckets, custom_tags)
            else:
                getattr(self, message.name)(message, **kwargs)
        except AttributeError as err:
            self.log.debug("Unable to handle metric: {} - error: {}".format(message.name, err))

    def poll(self, endpoint, pFormat=PrometheusFormat.PROTOBUF, headers={}):
        """
        Polls the metrics from the prometheus metrics endpoint provided.
        Defaults to the protobuf format, but can use the formats specified by
        the PrometheusFormat class.
        Custom headers can be added to the default headers.

        Returns the content-type of the response and the content of the reponse itself.
        """
        if 'accept-encoding' not in headers:
            headers['accept-encoding'] = 'gzip'
        if pFormat == PrometheusFormat.PROTOBUF:
            headers['accept'] = 'application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited'

        req = requests.get(endpoint, headers=headers)
        req.raise_for_status()
        return req.headers['Content-Type'], req.content

    def _submit(self, metric_name, message, send_histograms_buckets=True, custom_tags=None):
        """
        For each metric in the message, report it as a gauge with all labels as tags
        except if a labels dict is passed, in which case keys are label names we'll extract
        and corresponding values are tag names we'll use (eg: {'node': 'node'}).

        Histograms generate a set of values instead of a unique metric.
        send_histograms_buckets is used to specify if yes or no you want to
            send the buckets as tagged values when dealing with histograms.

        `custom_tags` is an array of 'tag:value' that will be added to the
        metric when sending the gauge to Datadog.
        """
        if message.type < len(self.METRIC_TYPES):
            for metric in message.metric:
                if message.type == 4:
                    self._submit_gauges_from_histogram(metric_name, metric, send_histograms_buckets, custom_tags)
                elif message.type == 2:
                    self._submit_gauges_from_summary(metric_name, metric, custom_tags)
                else:
                    val = getattr(metric, self.METRIC_TYPES[message.type]).value
                    self._submit_gauge(metric_name, val, metric, custom_tags)

        else:
            self.log.error("Metric type {} unsupported for metric {}.".format(message.type, message.name))

    def _submit_gauge(self, metric_name, val, metric, custom_tags=None):
        """
        Submit a metric as a gauge, additional tags provided will be added to
        the ones from the label provided via the metrics object.

        `custom_tags` is an array of 'tag:value' that will be added to the
        metric when sending the gauge to Datadog.
        """
        _tags = []
        if custom_tags is not None:
            _tags += custom_tags
        for label in metric.label:
            if self.exclude_labels is None or label.name not in self.exclude_labels:
                tag_name = label.name
                if self.labels_mapper is not None and label.name in self.labels_mapper:
                    tag_name = self.labels_mapper[label.name]
                _tags.append('{}:{}'.format(tag_name, label.value))
        self.gauge('{}.{}'.format(self.NAMESPACE, metric_name), val, _tags)

    def _submit_gauges_from_summary(self, name, metric, custom_tags=None):
        """
        Extracts metrics from a prometheus summary metric and sends them as gauges
        """
        if custom_tags is None:
            custom_tags = []
        # summaries do not have a value attribute
        val = getattr(metric, self.METRIC_TYPES[2]).sample_count
        self._submit_gauge("{}.count".format(name), val, metric, custom_tags)
        val = getattr(metric, self.METRIC_TYPES[2]).sample_sum
        self._submit_gauge("{}.sum".format(name), val, metric, custom_tags)
        for quantile in getattr(metric, self.METRIC_TYPES[2]).quantile:
            val = quantile.value
            limit = quantile.quantile
            self._submit_gauge("{}.quantile".format(name), val, metric, custom_tags=custom_tags+["quantile:{}".format(limit)])

    def _submit_gauges_from_histogram(self, name, metric, send_histograms_buckets=True, custom_tags=None):
        """
        Extracts metrics from a prometheus histogram and sends them as gauges
        """
        if custom_tags is None:
            custom_tags = []
        # histograms do not have a value attribute
        val = getattr(metric, self.METRIC_TYPES[4]).sample_count
        self._submit_gauge("{}.count".format(name), val, metric, custom_tags)
        val = getattr(metric, self.METRIC_TYPES[4]).sample_sum
        self._submit_gauge("{}.sum".format(name), val, metric, custom_tags)
        if send_histograms_buckets:
            for bucket in getattr(metric, self.METRIC_TYPES[4]).bucket:
                val = bucket.cumulative_count
                limit = bucket.upper_bound
                self._submit_gauge("{}.count".format(name), val, metric, custom_tags=custom_tags+["upper_bound:{}".format(limit)])
