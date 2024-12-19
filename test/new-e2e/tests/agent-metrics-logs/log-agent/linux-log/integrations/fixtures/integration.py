from datadog_checks.base import AgentCheck
from datadog_checks.base.utils.time import get_timestamp


class HelloCheck(AgentCheck):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.counter = 0  # Initialize increasing variable

    def check(self, instance):
        data = {}
        log_str = instance['log_message']
        data['timestamp'] = get_timestamp()
        data['ddtags'] = instance['integration_tags']

        log_str = instance['log_message']
        if instance['unique_message']:
            log_str = instance['log_message'] * instance['log_size']
            self.counter += 1
            log_str = "counter: " + str(self.counter) + ' ' + log_str
            data['message'] = log_str
        else:
            data['message'] = log_str

        num_logs = instance['log_count']

        if num_logs != 1:
            for _ in range(num_logs):
                self.send_log(data)
        else:
            self.send_log(data)
