try:
    from datadog_checks.base import AgentCheck
except ImportError:
    from checks import AgentCheck

__version__ = "1.0.0"


class TagCheckCheck(AgentCheck):
    """A minimal check that reports a gauge with instance-specific tags.

    Configuration example (two instances with different tags):

        instances:
          - metric_value: 1
            tags:
              - instance:alpha
          - metric_value: 2
            tags:
              - instance:beta
    """

    def check(self, instance):
        value = float(instance.get("metric_value", 0))
        tags = instance.get("tags", [])
        self.gauge("tag_check.metric", value, tags=tags)
        self.service_check("tag_check.can_connect", AgentCheck.OK, tags=tags)
