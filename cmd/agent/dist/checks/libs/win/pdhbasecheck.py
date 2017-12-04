# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

# project
from checks import AgentCheck
from utils.containers import hash_mutable
# datadog
try:
    from checks.libs.win.winpdh import WinPDHCounter
except ImportError:
    def WinPDHCounter(*args, **kwargs):
        return

class PDHBaseCheck(AgentCheck):
    """
    PDH based check.  check.

    Windows only.
    """
    def __init__(self, name, init_config, agentConfig, instances, counter_list):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances)
        self._countersettypes = {}
        self._counters = {}
        self._metrics = {}
        self._tags = {}

        try:
            for instance in instances:
                key = hash_mutable(instance)

                cfg_tags = instance.get('tags')
                if cfg_tags is not None:
                    tags = cfg_tags.join(",")
                    self._tags[key] = list(tags) if tags else []

                # list of the metrics.  Each entry is itself an entry,
                # which is the pdh name, datadog metric name, type, and the
                # pdh counter object
                self._metrics[key] = []
                for counterset, inst_name, counter_name, dd_name, mtype in counter_list:
                    m = getattr(self, mtype.lower())
                    obj = WinPDHCounter(counterset, counter_name, self.log, inst_name)
                    entry = [inst_name, dd_name, m, obj]
                    self.log.debug("entry: %s" % str(entry))
                    self._metrics[key].append(entry)

        except Exception as e:
            self.log.debug("Exception in PDH init: %s", str(e))
            raise

    def check(self, instance):
        key = hash_mutable(instance)
        for inst_name, dd_name, metric_func, counter in self._metrics[key]:
            try:
                vals = counter.get_all_values()
                for key, val in vals.iteritems():
                    tags = []
                    if key in self._tags:
                        tags = self._tags[key]

                    if not counter.is_single_instance():
                        tag = "instance=%s" % key
                        tags.append(tag)
                    metric_func(dd_name, val, tags)
            except Exception as e:
                # don't give up on all of the metrics because one failed
                self.log.error("Failed to get data for %s %s: %s" % (inst_name, dd_name, str(e)))
                pass
