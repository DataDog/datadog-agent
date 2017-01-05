from checks import AgentCheck


class TestCheck(AgentCheck):
    def __init__(self, name, init_config, instances):
        super(TestCheck, self).__init__(name, init_config, instances)

    def check(self, instance):
        pass
