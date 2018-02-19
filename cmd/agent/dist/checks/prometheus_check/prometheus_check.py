# (C) Datadog, Inc. 2018
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

import requests
from collections import defaultdict
from google.protobuf.internal.decoder import _DecodeVarint32  # pylint: disable=E0611,E0401
from checks import AgentCheck
from . import metrics_pb2

from prometheus_client.parser import text_fd_to_metric_families


# Prometheus check is a parent class providing a structure and some helpers
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
#   - create method named after the prometheus metric with the signature prometheus_metric_name(self, message, **kwargs)
#     it will be called in `process_metric`
#

# Used to specify if you want to use the protobuf format or the text format when
# querying prometheus metrics
class PrometheusFormat:
    PROTOBUF = "PROTOBUF"
    TEXT = "TEXT"


class UnknownFormatError(TypeError):
    pass


class PrometheusCheck(AgentCheck):
    UNWANTED_LABELS = ["le", "quantile"]  # are specifics keys for prometheus itself
    REQUESTS_CHUNK_SIZE = 1024 * 10  # use 10kb as chunk size when using the Stream feature in requests.get

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        # message.type is the index in this array
        # see: https://github.com/prometheus/client_model/blob/master/ruby/lib/prometheus/client/model/metrics.pb.rb
        self.METRIC_TYPES = ['counter', 'gauge', 'summary', 'untyped', 'histogram']

        # `NAMESPACE` is the prefix metrics will have. Need to be hardcoded in the
        # child check class.
        self.NAMESPACE = ''

        # `metrics_mapper` is a dictionary where the keys are the metrics to capture
        # and the values are the corresponding metrics names to have in datadog.
        # Note: it is empty in the parent class but will need to be
        # overloaded/hardcoded in the final check not to be counted as custom metric.
        self.metrics_mapper = {}

        # `label_joins` holds the configuration for extracting 1:1 labels from
        # a target metric to all metric matching the label, example:
        # self.label_joins = {
        #     'kube_pod_info': {
        #         'label_to_match': 'pod',
        #         'labels_to_get': ['node', 'host_ip']
        #     }
        # }
        self.label_joins = {}

        # `_label_mapping` holds the additionals label info to add for a specific
        # label value, example:
        # self._label_mapping = {
        #     'pod': {
        #         'dd-agent-9s1l1': [("node","yolo"),("host_ip","yey")]
        #     }
        # }
        self._label_mapping = {}

        # `_active_label_mapping` holds a dictionary of label values found during the run
        # to cleanup the label_mapping of unused values, example:
        # self._active_label_mapping = {
        #     'pod': {
        #         'dd-agent-9s1l1': True
        #     }
        # }
        self._active_label_mapping = {}

        # `_watched_labels` holds the list of label to watch for enrichment
        self._watched_labels = set()

        self._dry_run = True

        # Some metrics are ignored because they are duplicates or introduce a
        # very high cardinality. Metrics included in this list will be silently
        # skipped without a 'Unable to handle metric' debug line in the logs
        self.ignore_metrics = []

        # If the `labels_mapper` dictionary is provided, the metrics labels names
        # in the `labels_mapper` will use the corresponding value as tag name
        # when sending the gauges.
        self.labels_mapper = {}

        # `exclude_labels` is an array of labels names to exclude. Those labels
        # will just not be added as tags when submitting the metric.
        self.exclude_labels = []

        # `type_overrides` is a dictionary where the keys are prometheus metric names
        # and the values are a metric type (name as string) to use instead of the one
        # listed in the payload. It can be used to force a type on untyped metrics.
        # Note: it is empty in the parent class but will need to be
        # overloaded/hardcoded in the final check not to be counted as custom metric.
        self.type_overrides = {}

        # Some metrics are retrieved from differents hosts and often
        # a label can hold this information, this transfer it to the hostname
        self.label_to_hostname = None

        # Can either be only the path to the certificate and thus you should specify the private key
        # or it can be the path to a file containing both the certificate & the private key
        self.ssl_cert = None

        # Needed if the certificate does not include the private key
        #
        # /!\ The private key to your local certificate must be unencrypted.
        # Currently, Requests does not support using encrypted keys.
        self.ssl_private_key = None

        # The path to the trusted CA used for generating custom certificates
        self.ssl_ca_cert = None

    def check(self, instance):
        """
        check should take care of getting the url and other params
        from the instance and using the utils to process messages and submit metrics.
        """
        raise NotImplementedError()

    def parse_metric_family(self, response):
        """
        Parse the MetricFamily from a valid requests.Response object to provide a MetricFamily object (see [0])

        The text format uses iter_lines() generator.

        The protobuf format directly parse the response.content property searching for Prometheus messages of type
        MetricFamily [0] delimited by a varint32 [1] when the content-type is a `application/vnd.google.protobuf`.

        [0] https://github.com/prometheus/client_model/blob/086fe7ca28bde6cec2acd5223423c1475a362858/metrics.proto#L76-%20%20L81
        [1] https://developers.google.com/protocol-buffers/docs/reference/java/com/google/protobuf/AbstractMessageLite#writeDelimitedTo(java.io.OutputStream)

        :param response: requests.Response
        :return: metrics_pb2.MetricFamily()
        """
        if 'application/vnd.google.protobuf' in response.headers['Content-Type']:
            n = 0
            buf = response.content
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

        elif 'text/plain' in response.headers['Content-Type']:
            messages = defaultdict(list)  # map with the name of the element (before the labels)
            # and the list of occurrences with labels and values

            obj_map = {}  # map of the types of each metrics
            obj_help = {}  # help for the metrics
            for metric in text_fd_to_metric_families(response.iter_lines(chunk_size=self.REQUESTS_CHUNK_SIZE)):
                metric_name = "%s_bucket" % metric.name if metric.type == "histogram" else metric.name
                metric_type = self.type_overrides.get(metric_name, metric.type)
                if metric_type == "untyped" or metric_type not in self.METRIC_TYPES:
                    continue

                for sample in metric.samples:
                    if (sample[0].endswith("_sum") or sample[0].endswith("_count")) and \
                            metric_type in ["histogram", "summary"]:
                        messages[sample[0]].append({"labels": sample[1], 'value': sample[2]})
                    else:
                        messages[metric_name].append({"labels": sample[1], 'value': sample[2]})

                obj_map[metric.name] = metric_type
                obj_help[metric.name] = metric.documentation

            for _m in obj_map:
                if _m in messages or (obj_map[_m] == 'histogram' and ('{}_bucket'.format(_m) in messages)):
                    yield self._extract_metric_from_map(_m, messages, obj_map, obj_help)
        else:
            raise UnknownFormatError('Unsupported content-type provided: {}'.format(
                response.headers['Content-Type']))

    @staticmethod
    def get_metric_value_by_labels(messages, _metric, _m, metric_suffix):
        """
        :param messages: dictionary as metric_name: {labels: {}, value: 10}
        :param _metric: dictionary as {labels: {le: '0.001', 'custom': 'value'}}
        :param _m: str as metric name
        :param metric_suffix: str must be in (count or sum)
        :return: value of the metric_name matched by the labels
        """
        metric_name = '{}_{}'.format(_m, metric_suffix)
        expected_labels = set([(k, v) for k, v in _metric["labels"].iteritems()
                               if k not in PrometheusCheck.UNWANTED_LABELS])
        for elt in messages[metric_name]:
            current_labels = set([(k, v) for k, v in elt["labels"].iteritems()
                                  if k not in PrometheusCheck.UNWANTED_LABELS])
            # As we have two hashable objects we can compare them without any side effects
            if current_labels == expected_labels:
                return float(elt["value"])

        raise AttributeError("cannot find expected labels for metric %s with suffix %s" % (metric_name, metric_suffix))

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
                    _g.summary.sample_count = long(self.get_metric_value_by_labels(messages, _metric, _m, 'count'))
                if '{}_sum'.format(_m) in messages:
                    _g.summary.sample_sum = self.get_metric_value_by_labels(messages, _metric, _m, 'sum')
            # TODO: see what can be done with the untyped metrics
            elif obj_map[_m] == 'histogram':
                if '{}_count'.format(_m) in messages:
                    _g.histogram.sample_count = long(self.get_metric_value_by_labels(messages, _metric, _m, 'count'))
                if '{}_sum'.format(_m) in messages:
                    _g.histogram.sample_sum = self.get_metric_value_by_labels(messages, _metric, _m, 'sum')
            # last_metric = len(_obj.metric) - 1
            # if last_metric >= 0:
            for lbl in _metric['labels']:
                # In the string format, the quantiles are in the labels
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

    def process(self, endpoint, send_histograms_buckets=True, instance=None):
        """
        Polls the data from prometheus and pushes them as gauges
        `endpoint` is the metrics endpoint to use to poll metrics from Prometheus

        Note that if the instance has a 'tags' attribute, it will be pushed
        automatically as additionnal custom tags and added to the metrics
        """
        response = self.poll(endpoint)
        try:
            # no dry run if no label joins
            if not self.label_joins:
                self._dry_run = False
            elif not self._watched_labels:
                # build the _watched_labels set
                for metric, val in self.label_joins.iteritems():
                    self._watched_labels.add(val['label_to_match'])

            tags = []
            if instance is not None:
                tags = instance.get('tags', [])
            for metric in self.parse_metric_family(response):
                self.process_metric(metric, send_histograms_buckets=send_histograms_buckets, custom_tags=tags, instance=instance)

            # Set dry run off
            self._dry_run = False
            # Garbage collect unused mapping and reset active labels
            for metric, mapping in self._label_mapping.items():
                for key, val in mapping.items():
                    if key not in self._active_label_mapping[metric]:
                        del self._label_mapping[metric][key]
            self._active_label_mapping = {}
        finally:
            response.close()

    def process_metric(self, message, send_histograms_buckets=True, custom_tags=None, **kwargs):
        """
        Handle a prometheus metric message according to the following flow:
            - search self.metrics_mapper for a prometheus.metric <--> datadog.metric mapping
            - call check method with the same name as the metric
            - log some info if none of the above worked

        `send_histograms_buckets` is used to specify if yes or no you want to send the buckets as tagged values when dealing with histograms.
        """

        # If targeted metric, store labels
        if message.name in self.label_joins:
            matching_label = self.label_joins[message.name]['label_to_match']
            for metric in message.metric:
                labels_list = []
                matching_value = None
                for label in metric.label:
                    if label.name == matching_label:
                        matching_value = label.value
                    elif label.name in self.label_joins[message.name]['labels_to_get']:
                        labels_list.append((label.name, label.value))
                try:
                    self._label_mapping[matching_label][matching_value] = labels_list
                except KeyError:
                    if matching_value is not None:
                        self._label_mapping[matching_label] = {matching_value: labels_list}

        if message.name in self.ignore_metrics:
            return  # Ignore the metric

        # Filter metric to see if we can enrich with joined labels
        if self.label_joins:
            for metric in message.metric:
                for label in metric.label:
                    if label.name in self._watched_labels:
                        # Set this label value as active
                        if label.name not in self._active_label_mapping:
                            self._active_label_mapping[label.name] = {}
                        self._active_label_mapping[label.name][label.value] = True
                        # If mapping found add corresponding labels
                        try:
                            for label_tuple in self._label_mapping[label.name][label.value]:
                                extra_label = metric.label.add()
                                extra_label.name, extra_label.value = label_tuple
                        except KeyError:
                            pass

        try:
            if not self._dry_run:
                try:
                    self._submit(self.metrics_mapper[message.name], message, send_histograms_buckets, custom_tags)
                except KeyError:
                    getattr(self, message.name)(message, **kwargs)
        except AttributeError as err:
            self.log.debug("Unable to handle metric: {} - error: {}".format(message.name, err))

    def poll(self, endpoint, pFormat=PrometheusFormat.PROTOBUF, headers=None):
        """
        Polls the metrics from the prometheus metrics endpoint provided.
        Defaults to the protobuf format, but can use the formats specified by
        the PrometheusFormat class.
        Custom headers can be added to the default headers.

        Returns a valid requests.Response, raise requests.HTTPError if the status code of the requests.Response
        isn't valid - see response.raise_for_status()

        The caller needs to close the requests.Response

        :param endpoint: string url endpoint
        :param pFormat: the preferred format defined in PrometheusFormat
        :param headers: extra headers
        :return: requests.Response
        """
        if headers is None:
            headers = {}
        if 'accept-encoding' not in headers:
            headers['accept-encoding'] = 'gzip'
        if pFormat == PrometheusFormat.PROTOBUF:
            headers['accept'] = 'application/vnd.google.protobuf; ' \
                                'proto=io.prometheus.client.MetricFamily; ' \
                                'encoding=delimited'
        cert = None
        if isinstance(self.ssl_cert, basestring):
            cert = self.ssl_cert
            if isinstance(self.ssl_private_key, basestring):
                cert = (self.ssl_cert, self.ssl_private_key)
        verify = True
        if isinstance(self.ssl_ca_cert, basestring):
            verify = self.ssl_ca_cert
        try:
            response = requests.get(endpoint, headers=headers, stream=True, cert=cert, verify=verify)
        except (IOError, requests.exceptions.SSLError):
            self.log.error("Invalid SSL settings for requesting {} endpoint".format(endpoint))
            raise
        try:
            response.raise_for_status()
            return response
        except requests.HTTPError:
            response.close()
            raise

    def _submit(self, metric_name, message, send_histograms_buckets=True, custom_tags=None, hostname=None):
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
                custom_hostname = self._get_hostname(hostname, metric)
                if message.type == 4:
                    self._submit_gauges_from_histogram(metric_name, metric, send_histograms_buckets, custom_tags, custom_hostname)
                elif message.type == 2:
                    self._submit_gauges_from_summary(metric_name, metric, custom_tags, custom_hostname)
                else:
                    val = getattr(metric, self.METRIC_TYPES[message.type]).value
                    self._submit_gauge(metric_name, val, metric, custom_tags, custom_hostname)

        else:
            self.log.error("Metric type {} unsupported for metric {}.".format(message.type, message.name))

    def _get_hostname(self, hostname, metric):
        """
        If hostname is None, look at label_to_hostname setting
        """
        if hostname is None and self.label_to_hostname is not None:
            for label in metric.label:
                if label.name == self.label_to_hostname:
                    return label.value

        return hostname

    def _submit_gauge(self, metric_name, val, metric, custom_tags=None, hostname=None):
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
        self.gauge('{}.{}'.format(self.NAMESPACE, metric_name), val, _tags, hostname=hostname)

    def _submit_gauges_from_summary(self, name, metric, custom_tags=None, hostname=None):
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
            self._submit_gauge("{}.quantile".format(name), val, metric, custom_tags=custom_tags+["quantile:{}".format(limit)], hostname=hostname)

    def _submit_gauges_from_histogram(self, name, metric, send_histograms_buckets=True, custom_tags=None, hostname=None):
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
                self._submit_gauge("{}.count".format(name), val, metric, custom_tags=custom_tags+["upper_bound:{}".format(limit)], hostname=hostname)
