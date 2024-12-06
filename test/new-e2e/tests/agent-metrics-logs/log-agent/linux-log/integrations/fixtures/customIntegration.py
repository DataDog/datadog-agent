import uuid

from datadog_checks.base import AgentCheck
from datadog_checks.base.utils.time import get_timestamp


class HelloCheck(AgentCheck):
    def check(self, instance):
        data = {}
        log_str = instance['log_message']
        data['timestamp'] = get_timestamp()
        tags = instance['integration_tags']
        data['ddtags'] = tags

        log_str = instance['log_message'] * instance['log_size']
        if instance['unique_message']:
            uuid_str = str(uuid.uuid4())
            log_str = uuid_str + log_str
            data['message'] = log_str
        else:
            data['message'] = log_str

        num_logs = instance['log_count']

        for _ in range(num_logs):
            self.send_log(data)
