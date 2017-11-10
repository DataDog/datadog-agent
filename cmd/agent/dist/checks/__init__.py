# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

import copy
import json
import traceback
import re
import logging
import unicodedata
from collections import defaultdict

import aggregator
import datadog_agent
from config import (
    _is_affirmative,
    _get_py_loglevel
)
from utils.proxy import (
    get_requests_proxy,
    config_proxy_skip
)


class AgentLogHandler(logging.Handler):
    """
    This handler forwards every log to the Go backend allowing python checks to
    log message within the main agent logging system.
    """

    def emit(self, record):
        msg = self.format(record)
        datadog_agent.log("(%s:%s) | %s" % (record.filename, record.lineno, msg), record.levelno)

rootLogger = logging.getLogger()
rootLogger.addHandler(AgentLogHandler())
rootLogger.setLevel(_get_py_loglevel(datadog_agent.get_config('log_level')))

class CheckException(Exception):
    pass

class AgentCheck(object):
    OK, WARNING, CRITICAL, UNKNOWN = (0, 1, 2, 3)

    def __init__(self, *args, **kwargs):
        # `args` order is `name`, `init_config`, `agentConfig` (deprecated), `instances`

        self.metrics = defaultdict(list)

        self.instances = kwargs.get('instances', [])
        self.name = kwargs.get('name', '')
        self.init_config = kwargs.get('init_config', {})
        self.agentConfig = kwargs.get('agentConfig', {})
        self.warnings = []

        if len(args) > 0:
            self.name = args[0]
        if len(args) > 1:
            self.init_config = args[1]
        if len(args) > 2:
            if len(args) > 3 or 'instances' in kwargs:
                # old-style init: the 3rd argument is `agentConfig`
                self.agentConfig = args[2]
                if len(args) > 3:
                    self.instances = args[3]
            else:
                # new-style init: the 3rd argument is `instances`
                self.instances = args[2]

        self.hostname = datadog_agent.get_hostname()  # `self.hostname` is deprecated, use `datadog_agent.get_hostname()` instead

        # the agent5 'AgentCheck' setup a log attribute.
        self.log = logging.getLogger('%s.%s' % (__name__, self.name))

        # Set proxy settings
        self.proxies = get_requests_proxy(self.agentConfig)
        if not self.init_config:
            self._use_agent_proxy = True
        else:
            self._use_agent_proxy = _is_affirmative(
                self.init_config.get("use_agent_proxy", True))

        self.default_integration_http_timeout = float(self.agentConfig.get('default_integration_http_timeout', 9))

        self._deprecations = {
            'increment': [
                False,
                "DEPRECATION NOTICE: `AgentCheck.increment`/`AgentCheck.decrement` are deprecated, please use " +
                "`AgentCheck.gauge` or `AgentCheck.count` instead, with a different metric name",
            ],
            'device_name': [
                False,
                "DEPRECATION NOTICE: `device_name` is deprecated, please use a `device:` tag in the `tags` list instead",
            ],
            'in_developer_mode': [
                False,
                "DEPRECATION NOTICE: `in_developer_mode` is deprecated, please stop using it.",
            ],
        }


    @property
    def in_developer_mode(self):
        self._log_deprecation('in_developer_mode')
        return False


    def get_instance_proxy(self, instance, uri):
        proxies = self.proxies.copy()

        skip = _is_affirmative(instance.get('no_proxy', not self._use_agent_proxy))
        return config_proxy_skip(proxies, uri, skip)

    def _submit_metric(self, mtype, name, value, tags=None, hostname=None, device_name=None):
        if value is None:
            # ignore metric sample
            return

        tags = self._normalize_tags(tags, device_name)
        if hostname is None:
            hostname = ""

        aggregator.submit_metric(self, self.check_id, mtype, name, float(value), tags, hostname)

    def gauge(self, name, value, tags=None, hostname=None, device_name=None):
        self._submit_metric(aggregator.GAUGE, name, value, tags=tags, hostname=hostname, device_name=device_name)

    def count(self, name, value, tags=None, hostname=None, device_name=None):
        self._submit_metric(aggregator.COUNT, name, value, tags=tags, hostname=hostname, device_name=device_name)

    def monotonic_count(self, name, value, tags=None, hostname=None, device_name=None):
        self._submit_metric(aggregator.MONOTONIC_COUNT, name, value, tags=tags, hostname=hostname, device_name=device_name)

    def rate(self, name, value, tags=None, hostname=None, device_name=None):
        self._submit_metric(aggregator.RATE, name, value, tags=tags, hostname=hostname, device_name=device_name)

    def histogram(self, name, value, tags=None, hostname=None, device_name=None):
        self._submit_metric(aggregator.HISTOGRAM, name, value, tags=tags, hostname=hostname, device_name=device_name)

    def historate(self, name, value, tags=None, hostname=None, device_name=None):
        self._submit_metric(aggregator.HISTORATE, name, value, tags=tags, hostname=hostname, device_name=device_name)

    def increment(self, name, value=1, tags=None, hostname=None, device_name=None):
        self._log_deprecation("increment")
        self._submit_metric(aggregator.COUNTER, name, value, tags=tags, hostname=hostname, device_name=device_name)

    def decrement(self, name, value=-1, tags=None, hostname=None, device_name=None):
        self._log_deprecation("increment")
        self._submit_metric(aggregator.COUNTER, name, value, tags=tags, hostname=hostname, device_name=device_name)

    def _log_deprecation(self, deprecation_key):
        """
        Logs a deprecation notice at most once per AgentCheck instance, for the pre-defined `deprecation_key`
        """
        if not self._deprecations[deprecation_key][0]:
            self.log.warning(self._deprecations[deprecation_key][1])
            self._deprecations[deprecation_key][0] = True

    def service_check(self, name, status, tags=None, hostname=None, message=None):
        tags = self._normalize_tags_type(tags)
        if hostname is None:
            hostname = ""
        if message is None:
            message = ""

        aggregator.submit_service_check(self, self.check_id, name, status, tags, hostname, message)

    def event(self, event):
        # Enforce types of some fields, considerably facilitates handling in go bindings downstream
        for key, value in event.items():
            # transform the unicode objects to plain strings with utf-8 encoding
            if isinstance(value, unicode):
                try:
                    event[key] = event[key].encode('utf-8')
                except UnicodeError:
                    self.log.warning("Error encoding unicode field '%s' of event to utf-8 encoded string, can't submit event", key)
                    return
        if event.get('tags'):
            event['tags'] = self._normalize_tags_type(event['tags'])
        if event.get('timestamp'):
            event['timestamp'] = int(event['timestamp'])
        if event.get('aggregation_key'):
            event['aggregation_key'] = str(event['aggregation_key'])
        aggregator.submit_event(self, self.check_id, event)

    # TODO(olivier): implement service_metadata if it's worth it
    def service_metadata(self, meta_name, value):
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
            metric_name = unicodedata.normalize('NFKD', metric).encode('ascii', 'ignore')
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

    def _normalize_tags(self, tags, device_name):
        """
        Normalize tags:
        - append `device_name` as `device:` tag
        - normalize tags to type `str`
        - always return a list
        """
        if tags is None:
            normalized_tags = []
        else:
            normalized_tags = list(tags)  # normalize to `list` type, and make a copy

        if device_name:
            self._log_deprecation("device_name")
            normalized_tags.append("device:%s" % device_name)

        return self._normalize_tags_type(normalized_tags)

    def _normalize_tags_type(self, tags):
        """
        Normalize all the tags to strings (type `str`) so that the go bindings can handle them easily
        Doesn't mutate the passed list, returns a new list
        """
        normalized_tags = []
        if tags is not None:
            for tag in tags:
                if not isinstance(tag, basestring):
                    try:
                        tag = str(tag)
                    except Exception:
                        self.log.warning("Error converting tag to string, ignoring tag")
                        continue
                elif isinstance(tag, unicode):
                    try:
                        tag = tag.encode('utf-8')
                    except UnicodeError:
                        self.log.warning("Error encoding unicode tag to utf-8 encoded string, ignoring tag")
                        continue
                normalized_tags.append(tag)

        return normalized_tags

    def warning(self, warning_message):
        warning_message = str(warning_message)
        self.log.warning(warning_message)
        self.warnings.append(warning_message)

    def get_warnings(self):
        """
        Return the list of warnings messages to be displayed in the info page
        """
        warnings = self.warnings
        self.warnings = []
        return warnings

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
