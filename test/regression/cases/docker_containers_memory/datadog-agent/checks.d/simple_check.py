import datadog_agent
from datadog_checks.base import AgentCheck


class SimpleCheck(AgentCheck):
    def __init__(self, name, init_config, instances):
        super().__init__(name, init_config, instances)
        self.tags = self.instance.get("tags", [])

    def check(self, instance):
        self.gauge("simple.value", 42, tags=self.tags)
        datadog_agent.emit_agent_telemetry("simple_check", "run", 1, "counter")
