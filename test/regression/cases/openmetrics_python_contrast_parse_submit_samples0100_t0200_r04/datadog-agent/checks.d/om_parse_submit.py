from datadog_checks.checks import AgentCheck
from prometheus_client.parser import text_string_to_metric_families


class OmParseSubmit(AgentCheck):
    def check(self, instance):
        response = self.http.get(instance['endpoint'])
        submitted = 0
        for family in text_string_to_metric_families(response.text):
            for sample in family.samples:
                self.gauge('openmetrics.python_contrast.sample', sample.value)
                submitted += 1
        self.gauge('openmetrics.python_contrast.submitted', submitted)
