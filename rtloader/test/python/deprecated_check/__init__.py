from datadog_checks.base.checks import AgentCheck


class DeprecatedCheck(AgentCheck):

    def __init__(self, name, init_config, agentConfig, instances=None):
        AgentCheck.__init__(self, name, init_config, agentConfig, instances=instances)


__version__ = '0.0.1'
