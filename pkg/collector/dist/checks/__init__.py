import json
import traceback
import re
import time
import copy
import logging
from collections import defaultdict

import aggregator


class AgentCheck(object):

    RATE = "rate"
    GAUGE = "gauge"
    OK = 0
    WARNING = 1
    CRITICAL = 2

    def __init__(self, *args, **kwargs):
        self.metrics = defaultdict(list)
        self.instances = kwargs.get('instances', [])
        # the agent5 'AgentCheck' setup a log attribute.
        self.log = logging.getLogger('%s.%s' % (__name__, kwargs.get("name", "")))

    def gauge(self, name, value, tags=None):
        aggregator.submit_data(self, aggregator.GAUGE, name, value, tags)

    def rate(self, name, value, tags=None):
        aggregator.submit_data(self, aggregator.RATE, name, value, tags)

    def histogram(self, name, value, tags=None, hostname=None, device_name=None):
        aggregator.submit_data(self, aggregator.HISTOGRAM, name, value, tags)

    def service_check(self, *args, **kwargs):
        pass

    def check(self, instance):
        raise NotImplementedError

    def normalize(self, metric, prefix=None, fix_case=False):
        """
        Turn a metric into a well-formed metric name
        prefix.b.c
        :param metric The metric name to normalize
        :param prefix A prefix to to add to the normalized name, default None
        :param fix_case A boolean, indicating whether to make sure that
                        the metric name returned is in underscore_case
        """
        if isinstance(metric, unicode):
            metric_name = unicodedata.normalize('NFKD', metric).encode('ascii','ignore')
        else:
            metric_name = metric

        if fix_case:
            name = self.convert_to_underscore_separated(metric_name)
            if prefix is not None:
                prefix = self.convert_to_underscore_separated(prefix)
        else:
            name = re.sub(r"[,\+\*\-/()\[\]{}\s]", "_", metric_name)
        # Eliminate multiple _
        name = re.sub(r"__+", "_", name)
        # Don't start/end with _
        name = re.sub(r"^_", "", name)
        name = re.sub(r"_$", "", name)
        # Drop ._ and _.
        name = re.sub(r"\._", ".", name)
        name = re.sub(r"_\.", ".", name)

        if prefix is not None:
            return prefix + "." + name
        else:
            return name

    FIRST_CAP_RE = re.compile('(.)([A-Z][a-z]+)')
    ALL_CAP_RE = re.compile('([a-z0-9])([A-Z])')
    METRIC_REPLACEMENT = re.compile(r'([^a-zA-Z0-9_.]+)|(^[^a-zA-Z]+)')
    DOT_UNDERSCORE_CLEANUP = re.compile(r'_*\._*')

    def convert_to_underscore_separated(self, name):
        """
        Convert from CamelCase to camel_case
        And substitute illegal metric characters
        """
        metric_name = self.FIRST_CAP_RE.sub(r'\1_\2', name)
        metric_name = self.ALL_CAP_RE.sub(r'\1_\2', metric_name).lower()
        metric_name = self.METRIC_REPLACEMENT.sub('_', metric_name)
        return self.DOT_UNDERSCORE_CLEANUP.sub('.', metric_name).strip('_')

    def warning(self, *args, **kwargs):
        pass

    def run(self):
        try:
            self.check(copy.deepcopy(self.instances[0]))
            result = ''

        except Exception, e:
            result = json.dumps([
                {
                    "message": str(e),
                    "traceback": traceback.format_exc(),
                }
            ])

        return result
