from datadog_checks.base.checks import AgentCheck

# Fake check for testing purposes
class FakeCheck(AgentCheck):
    def get_warnings(self):
        return ["warning 1", "warning 2", "warning 3"]


__version__ = '0.4.2'
