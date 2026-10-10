# the following try/except block will make the custom check compatible with any Agent version
try:
    # first, try to import the base class from new versions of the Agent...
    from datadog_checks.base import AgentCheck
except ImportError:
    # ...if the above failed, the check is running in Agent version < 6.6.0
    from checks import AgentCheck

# content of the special variable __version__ will be shown in the Agent status page
__version__ = "1.0.0"


class CustomInfraModeEventsCheck(AgentCheck):
    def check(self, instance):
        # Gauge is only here so the suite can assert custom_* metrics stay unmarked.
        self.gauge(
            "custom_infra_mode_events.metric",
            1,
            tags=["e2e:infra_mode_check_events"],
        )
        self.event(
            {
                "msg_title": "infra mode check event",
                "msg_text": "emitted by custom_infra_mode_events e2e check",
                "alert_type": "info",
                "source_type_name": "custom_infra_mode_events",
                "tags": ["e2e:infra_mode_check_events"],
            }
        )
