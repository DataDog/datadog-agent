from agent import AgentCheck


class TestCheck(AgentCheck):
    def __init__(self, name, init_config, agentConfig, instances):
        super(TestCheck, self).__init__(name, init_config, agentConfig, instances)

    def check(self, instance):
        pass
