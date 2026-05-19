"""High-volume Python AgentCheck used to stress CheckSampler windowing.

The check emits a stable set of contexts across the AgentCheck metric
submission paths. The 1Hz cadence exercises check metric window roll-up and AD
observer storage on the same workload.
"""

import aggregator
import datadog_agent
from datadog_checks.base import AgentCheck


class VolumeCheck(AgentCheck):
    def __init__(self, name, init_config, instances):
        super().__init__(name, init_config, instances)
        self.contexts_per_run = int(self.instance.get("contexts_per_run", 200))
        self.name_prefix = self.instance.get("name_prefix", "vc")
        self.base_tags = list(self.instance.get("base_tags", []))

    def check(self, _):
        prefix = self.name_prefix
        for i in range(self.contexts_per_run):
            tags = self.base_tags + ["context:%d" % i]
            metric_type = i % 10
            if metric_type == 0:
                self.gauge("%s.gauge" % prefix, float(i), tags=tags)
            elif metric_type == 1:
                self.rate("%s.rate" % prefix, float(i + 1), tags=tags)
            elif metric_type == 2:
                self.count("%s.count" % prefix, 1, tags=tags)
            elif metric_type == 3:
                self.monotonic_count("%s.monotonic_count" % prefix, float(i + 1), tags=tags)
            elif metric_type == 4:
                self.counter("%s.counter" % prefix, float(i + 1), tags=tags)
            elif metric_type == 5:
                self.set("%s.set" % prefix, "value:%d" % (i % 100), tags=tags)
            elif metric_type == 6:
                self.histogram("%s.histogram" % prefix, float(i), tags=tags)
            elif metric_type == 7:
                self.historate("%s.historate" % prefix, float(i + 1), tags=tags)
            elif metric_type == 8:
                self.distribution("%s.distribution" % prefix, float(i), tags=tags)
            else:
                for bucket in range(5):
                    aggregator.submit_histogram_bucket(
                        self,
                        self.check_id,
                        "%s.histogram_bucket" % prefix,
                        1,
                        float(bucket),
                        float(bucket + 1),
                        0,
                        "",
                        tags,
                        True,
                    )
        datadog_agent.emit_agent_telemetry("volume_check", "run", 1, "counter")
