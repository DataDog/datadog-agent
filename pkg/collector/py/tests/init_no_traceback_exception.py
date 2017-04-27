import aggregator
from checks import AgentCheck

class TestCheck(AgentCheck):
    def __init__(self, name, init_config, agentConfig, instances, one, two, three): # __init__ with many arguments
        pass

    def check(self, instance):
        pass
