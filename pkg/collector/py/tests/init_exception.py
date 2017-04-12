import aggregator
from checks import AgentCheck

class TestCheck(AgentCheck):
    def __init__(self, name, init_config, agentConfig, instances):
        raise RuntimeError("unexpected error")

    def check(self, instance):
        pass
