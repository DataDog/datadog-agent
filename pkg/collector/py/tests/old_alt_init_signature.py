from checks import AgentCheck


class TestCheck(AgentCheck):
    def __init__(self, name, init_config, agentConfig, instances=None):
        super(TestCheck, self).__init__(name, init_config, agentConfig, instances=instances)
        assert hasattr(self, 'instances')
        assert isinstance(self.instances, list)
        assert len(self.instances) > 0
        assert 'foo' in self.instances[0]

    def check(self, instance):
        pass
