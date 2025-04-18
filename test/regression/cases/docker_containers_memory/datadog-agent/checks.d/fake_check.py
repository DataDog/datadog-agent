import datadog_agent
from datadog_checks.base import AgentCheck


class FakeCheck(AgentCheck):
    def __init__(self, name, init_config, instances):
        super().__init__(name, init_config, instances)
        self.tags = self.instance.get("tags", [])

    def check(self, instance):
        self.gauge("fake.value", 42, tags=self.tags)
        datadog_agent.emit_agent_telemetry("fake_check", "run", 1, "counter")
