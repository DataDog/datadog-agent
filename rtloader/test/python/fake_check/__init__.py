from datadog_checks.base.checks import AgentCheck

was_canceled = False
discover_config_return = "[]"
discover_config_exception = None
discover_config_service_json = None


# Fake check for testing purposes
class FakeCheck(AgentCheck):
    @classmethod
    def discover_config(cls, service_json):
        global discover_config_service_json
        discover_config_service_json = service_json
        if discover_config_exception is not None:
            raise Exception(discover_config_exception)
        return discover_config_return

    def cancel(self):
        global was_canceled
        assert not was_canceled
        was_canceled = True

    def get_warnings(self):
        return ["warning 1", "warning 2", "warning 3"]


__version__ = '0.4.2'
