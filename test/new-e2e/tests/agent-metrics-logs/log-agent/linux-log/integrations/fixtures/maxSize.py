from datadog_checks.base import AgentCheck


class HelloCheck(AgentCheck):
    def check(self, instance):
        data = {}
        # Create a 1 MB string
        string_one_mb = "a" * 1024 * 1024
        # Remove 16 characters to account for the json the launcher adds and the
        # extra newline character
        string_one_mb = string_one_mb[:-15]
        data['message'] = string_one_mb

        self.send_log(data)
