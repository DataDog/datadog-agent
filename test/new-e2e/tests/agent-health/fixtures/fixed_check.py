from datadog_checks.base import AgentCheck


class BrokenCheck(AgentCheck):
    def check(self, instance):
        self.gauge("e2e.healthplatform.check_ok", 1)
