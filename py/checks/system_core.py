# 3p
import psutil

# project
from checks import AgentCheck


class SystemCore(AgentCheck):

    def check(self, instance):
        cpu_times = psutil.cpu_times(percpu=True)
        self.gauge("system.core.count", len(cpu_times))

        for i, cpu in enumerate(cpu_times):
            for key, value in cpu._asdict().iteritems():
                self.rate(
                    "system.core.{0}".format(key),
                    100.0 * value,
                    tags=["core:{0}".format(i)]
                )
