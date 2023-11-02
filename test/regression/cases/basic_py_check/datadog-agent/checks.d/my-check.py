from datadog_checks.checks import AgentCheck

class MyCheck(AgentCheck):
    def check(self, instance):
        self.gauge('hello.world', 1.23, tags=['foo:bar'])
