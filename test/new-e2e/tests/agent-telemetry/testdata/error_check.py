from datadog_checks.base import AgentCheck


class ErrorCheck(AgentCheck):
    def check(self, instance):
        self.log.error("intentional error for error tracking test")
