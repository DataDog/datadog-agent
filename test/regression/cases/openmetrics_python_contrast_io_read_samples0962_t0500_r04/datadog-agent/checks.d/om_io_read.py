from datadog_checks.checks import AgentCheck


class OmIoRead(AgentCheck):
    def check(self, instance):
        response = self.http.get(instance['endpoint'])
        self.gauge('openmetrics.python_contrast.bytes', len(response.text))
