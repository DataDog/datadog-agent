from datadog_checks.base.checks import AgentCheck


class TestCheck(AgentCheck):

    def __init__(self, name, init_config, instances):
        AgentCheck.__init__(self, name, init_config, instances)
        # Create a local variable to make sure we filter it
        agentConfig = True


__version__ = '0.0.1'
