# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2018 Datadog, Inc.

# project
from checks import AgentCheck
from utils.containers import hash_mutable
# datadog
try:
    from checks.libs.win.winpdh import WinPDHCounter, DATA_TYPE_INT, DATA_TYPE_DOUBLE
except ImportError:
    def WinPDHCounter(*args, **kwargs):
        return

import win32wnet

int_types = [
    "int",
    "long",
    "uint",
]

double_types = [
    "double",
    "float",
]
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
                    if not isinstance(cfg_tags, list):
                        self.log.error("Tags must be configured as a list")
                        raise ValueError("Tags must be type list, not %s" % str(type(cfg_tags)))
                    self._tags[key] = list(cfg_tags)

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

                ## counter_data_types allows the precision with which counters are queried
                ## to be configured on a per-metric basis. In the metric instance, precision
                ## should be specified as
                ## counter_data_types:
                ## - iis.httpd_request_method.get,int
                ## - iis.net.bytes_rcvd,float
                ##
                ## the above would query the counter associated with iis.httpd_request_method.get
                ## as an integer (LONG) and iis.net.bytes_rcvd as a double
                datatypes = {}
                precisions = instance.get('counter_data_types')
                if precisions is not None:
                    if not isinstance(precisions, list):
                        self.log.warning("incorrect type for counter_data_type %s" % str(precisions))
                    else:
                        for p in precisions:
                            k, v = p.split(",")
                            v = v.lower().strip()
                            if v in int_types:
                                self.log.info("Setting datatype for %s to integer" % k)
                                datatypes[k] = DATA_TYPE_INT
                            elif v in double_types:
                                self.log.info("Setting datatype for %s to double" % k)
                                datatypes[k] = DATA_TYPE_DOUBLE
                            else:
                                self.log.warning("Unknown data type %s" % str(v))

                # list of the metrics.  Each entry is itself an entry,
                # which is the pdh name, datadog metric name, type, and the
                # pdh counter object

                for counterset, inst_name, counter_name, dd_name, mtype in counter_list:
                    m = getattr(self, mtype.lower())

                    precision = datatypes.get(dd_name)

                    obj = WinPDHCounter(counterset, counter_name, self.log, inst_name, machine_name = remote_machine, precision=precision)
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

                        precision = datatypes.get(dd_name)

                        obj = WinPDHCounter(counterset, counter_name, self.log, inst_name, machine_name = remote_machine, precision = precision)
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
                        tags = list(self._tags[key])

                    if not counter.is_single_instance():
                        tag = "instance:%s" % instance_name
                        tags.append(tag)
                    metric_func(dd_name, val, tags)
            except Exception as e:
                # don't give up on all of the metrics because one failed
                self.log.error("Failed to get data for %s %s: %s" % (inst_name, dd_name, str(e)))
                pass
