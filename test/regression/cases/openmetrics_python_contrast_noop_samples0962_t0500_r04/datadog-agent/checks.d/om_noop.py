from datadog_checks.checks import AgentCheck


class OmNoop(AgentCheck):
    def check(self, _instance):
        self.gauge('openmetrics.python_contrast.noop', 1)
