from datadog_checks.base.checks import AgentCheck

was_canceled = False


# Fake check for testing purposes
class FakeCheck(AgentCheck):
    def cancel(self):
        global was_canceled
        assert not was_canceled
        was_canceled = True

    def get_warnings(self):
        return ["warning 1", "warning 2", "warning 3"]


__version__ = '0.4.2'
