from datadog_checks.base import AgentCheck


class ErrorCheck(AgentCheck):
    def check(self, instance):
        self.log.error("intentional error for error tracking test")
        raise ValueError("intentional core error for error tracking test")
