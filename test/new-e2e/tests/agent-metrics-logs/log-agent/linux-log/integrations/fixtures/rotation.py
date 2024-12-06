import uuid

from datadog_checks.base import AgentCheck


class HelloCheck(AgentCheck):
    def __init__(self, name, init_config, instances):
        super().__init__(name, init_config, instances)
        # Initialize a local variable to simulate a counter
        self.counter = 0

    def check(self, instance):
        self.counter = self.counter + 1
        data = {}

        uuid_str = str(uuid.uuid4())
        # Create a 256kb string
        total_size = instance["log_size"]
        # Remove 37 characters to account for the json characters, tags, and
        # newline character the launcher adds to the log
        padding_size = total_size - len(uuid_str) - 37
        log_str = uuid_str + ('a' * padding_size)
        data['message'] = log_str
        data['ddtags'] = instance['integration_tags']

        self.send_log(data)
