from datadog_checks.base import AgentCheck


class BrokenCheck(AgentCheck):
    def check(self, instance):
        raise Exception("synthetic failure for e2e health platform test")
