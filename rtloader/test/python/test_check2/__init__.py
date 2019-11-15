from datadog_checks.base.checks import AgentCheck


class TestCheck2(AgentCheck):

    def __init__(self, *args, **kwargs):
        AgentCheck.__init__(self, *args, **kwargs)
        # Create a local variable to make sure we filter it
        agentConfig = True


__version__ = '0.0.1'
