from datadog_checks.base import AgentCheck


class ErrorCheck(AgentCheck):
    def check(self, instance):
        # this triggers an error log from python code
        self.log.error("intentional error for error tracking test")
        # this triggers an error log from go code, in the error handler
        raise ValueError("intentional core error for error tracking test")
