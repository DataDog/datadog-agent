from checks import AgentCheck

class TestCheck(AgentCheck):
    def check(self, instance):
        self.gauge('foo', 'bar')
        self.gauge('foo', 'baz')
