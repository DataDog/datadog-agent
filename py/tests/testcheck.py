from checks import AgentCheck

class TestCheck(AgentCheck):
    def check(self, instance):
        self.gauge('foo', 0)
        self.gauge('foo', 1)
