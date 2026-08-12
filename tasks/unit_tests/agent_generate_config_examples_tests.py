import unittest
from unittest.mock import MagicMock, patch

from tasks.agent import generate_config_examples
from tasks.flavor import AgentFlavor
from tasks.schema.template import CORE_SCHEMA_FILE, SYSPROBE_SCHEMA_FILE


class TestGenerateConfigExamples(unittest.TestCase):
    @patch("tasks.agent.refresh_assets")
    @patch("tasks.agent.generate_template")
    @patch("tasks.agent.sys")
    def test_linux_base_flavor_renders_agent_and_sysprobe(self, sys_mod, gen, refresh):
        sys_mod.platform = "linux"
        ctx = MagicMock()

        generate_config_examples(
            ctx,
            flavor=AgentFlavor.base,
            skip_assets=True,
            build_tags=["python"],
            development=False,
            windows_sysprobe=False,
        )

        gen.assert_any_call(CORE_SCHEMA_FILE, "./cmd/agent/dist/datadog.yaml", "agent-py3", "linux")
        gen.assert_any_call(SYSPROBE_SCHEMA_FILE, "./cmd/agent/dist/system-probe.yaml", "system-probe", "linux")
        self.assertEqual(gen.call_count, 2)
        refresh.assert_not_called()

    @patch("tasks.agent.refresh_assets")
    @patch("tasks.agent.generate_template")
    @patch("tasks.agent.sys")
    def test_iot_flavor_uses_iot_agent_build_type(self, sys_mod, gen, _refresh):
        sys_mod.platform = "linux"
        ctx = MagicMock()

        generate_config_examples(
            ctx,
            flavor=AgentFlavor.iot,
            skip_assets=True,
            build_tags=[],
            development=False,
            windows_sysprobe=False,
        )

        gen.assert_any_call(CORE_SCHEMA_FILE, "./cmd/agent/dist/datadog.yaml", "iot-agent", "linux")

    @patch("tasks.agent.refresh_assets")
    @patch("tasks.agent.generate_template")
    @patch("tasks.agent.sys")
    def test_windows_skips_sysprobe_unless_requested(self, sys_mod, gen, _refresh):
        sys_mod.platform = "win32"
        ctx = MagicMock()

        generate_config_examples(
            ctx,
            flavor=AgentFlavor.base,
            skip_assets=True,
            build_tags=[],
            development=False,
            windows_sysprobe=False,
        )

        gen.assert_called_once_with(CORE_SCHEMA_FILE, "./cmd/agent/dist/datadog.yaml", "agent-py3", "windows")

    @patch("tasks.agent.refresh_assets")
    @patch("tasks.agent.generate_template")
    @patch("tasks.agent.sys")
    def test_windows_with_sysprobe_renders_sysprobe(self, sys_mod, gen, _refresh):
        sys_mod.platform = "win32"
        ctx = MagicMock()

        generate_config_examples(
            ctx,
            flavor=AgentFlavor.base,
            skip_assets=True,
            build_tags=[],
            development=False,
            windows_sysprobe=True,
        )

        gen.assert_any_call(SYSPROBE_SCHEMA_FILE, "./cmd/agent/dist/system-probe.yaml", "system-probe", "windows")
        self.assertEqual(gen.call_count, 2)

    @patch("tasks.agent.refresh_assets")
    @patch("tasks.agent.generate_template")
    @patch("tasks.agent.sys")
    def test_skip_assets_false_calls_refresh_assets(self, sys_mod, _gen, refresh):
        sys_mod.platform = "linux"
        ctx = MagicMock()

        generate_config_examples(
            ctx,
            flavor=AgentFlavor.base,
            skip_assets=False,
            build_tags=["python"],
            development=True,
            windows_sysprobe=False,
        )

        refresh.assert_called_once_with(
            ctx, ["python"], development=True, flavor=AgentFlavor.base.name, windows_sysprobe=False
        )


if __name__ == "__main__":
    unittest.main()
