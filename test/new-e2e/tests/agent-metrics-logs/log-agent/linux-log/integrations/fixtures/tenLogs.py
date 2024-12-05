from datadog_checks.base import AgentCheck
from datadog_checks.base.utils.time import get_timestamp


class HelloCheck(AgentCheck):
    def check(self, instance):
        log_str = instance['log_message']
        data = {}
        data['timestamp'] = get_timestamp()
        data['message'] = log_str
        tags = instance['integration_tags']
        data['ddtags'] = tags

        num_logs = instance['log_count']

        for _ in range(num_logs):
            self.send_log(data)
