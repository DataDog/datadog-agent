# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

# project
from checks import AgentCheck
from utils.containers import hash_mutable
# datadog
try:
    from checks.libs.win.winpdh import WinPDHCounter
except ImportError:
    def WinPDHCounter(*args, **kwargs):
        return

import win32wnet


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
                    tags = ",".join(cfg_tags)
                    self._tags[key] = list(tags) if tags else []
                remote_machine = None
                host = instance.get('host')
                self._metrics[key] = []
                if host is not None and host != ".":
                    try:
                        remote_machine = host

                        username = instance.get('username')
                        password = instance.get('password')
                        nr = win32wnet.NETRESOURCE()
                        nr.lpRemoteName = r"\\%s\c$" % remote_machine
                        nr.dwType = 0
                        nr.lpLocalName = None
                        win32wnet.WNetAddConnection2(nr, password, username, 0)

                    except Exception as e:
                        self.log.error("Failed to make remote connection %s" % str(e))
                        return

                # list of the metrics.  Each entry is itself an entry,
                # which is the pdh name, datadog metric name, type, and the
                # pdh counter object

                for counterset, inst_name, counter_name, dd_name, mtype in counter_list:
                    m = getattr(self, mtype.lower())
                    obj = WinPDHCounter(counterset, counter_name, self.log, inst_name, machine_name=remote_machine)
                    entry = [inst_name, dd_name, m, obj]
                    self.log.debug("entry: %s" % str(entry))
                    self._metrics[key].append(entry)

                # get any additional metrics in the instance
                addl_metrics = instance.get('additional_metrics')
                if addl_metrics is not None:
                    for counterset, inst_name, counter_name, dd_name, mtype in addl_metrics:
                        if inst_name.lower() == "none" or len(inst_name) == 0 or inst_name == "*" or inst_name.lower() == "all":
                            inst_name = None
                        m = getattr(self, mtype.lower())
                        obj = WinPDHCounter(counterset, counter_name, self.log, inst_name, machine_name=remote_machine)
                        entry = [inst_name, dd_name, m, obj]
                        self.log.debug("additional metric entry: %s" % str(entry))
                        self._metrics[key].append(entry)


        except Exception as e:
            self.log.debug("Exception in PDH init: %s", str(e))
            raise

    def check(self, instance):
        self.log.debug("PDHBaseCheck: check()")
        key = hash_mutable(instance)
        for inst_name, dd_name, metric_func, counter in self._metrics[key]:
            try:
                vals = counter.get_all_values()
                for instance_name, val in vals.iteritems():
                    tags = []
                    if key in self._tags:
                        tags = self._tags[key]

                    if not counter.is_single_instance():
                        tag = "instance:%s" % instance_name
                        tags.append(tag)
                    metric_func(dd_name, val, tags)
            except Exception as e:
                # don't give up on all of the metrics because one failed
                self.log.error("Failed to get data for %s %s: %s" % (inst_name, dd_name, str(e)))
                pass
