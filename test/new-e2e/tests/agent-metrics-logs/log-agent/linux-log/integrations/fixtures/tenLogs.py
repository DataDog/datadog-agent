from datadog_checks.base import AgentCheck
from datadog_checks.base.utils.time import get_timestamp


class HelloCheck(AgentCheck):
    def check(self, instance):
        data = {}
        data['timestamp'] = get_timestamp()
        data['message'] = "Custom log message"
        data['ddtags'] = "env:dev,bar:foo"

        for _ in range(10):
            self.send_log(data)
