import unittest
from unittest.mock import MagicMock, patch

from invoke.context import Context

from tasks.agent import generate_config_examples, refresh_assets
from tasks.flavor import AgentFlavor
from tasks.schema.template import CORE_SCHEMA_FILE, SYSPROBE_SCHEMA_FILE


class TestGenerateConfigExamples(unittest.TestCase):
    @patch("tasks.agent.refresh_assets")
    @patch("tasks.agent.generate_template")
    @patch("tasks.agent.get_target_platform", return_value="linux")
    def test_linux_base_flavor_renders_agent_and_sysprobe(self, _target_platform, gen, refresh):
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
    @patch("tasks.agent.get_target_platform", return_value="linux")
    def test_iot_flavor_uses_iot_agent_build_type(self, _target_platform, gen, _refresh):
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
    @patch("tasks.agent.get_target_platform", return_value="win32")
    def test_windows_skips_sysprobe_unless_requested(self, _target_platform, gen, _refresh):
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
    @patch("tasks.agent.get_target_platform", return_value="win32")
    def test_windows_with_sysprobe_renders_sysprobe(self, _target_platform, gen, _refresh):
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
    @patch("tasks.agent.get_target_platform", return_value="linux")
    def test_skip_assets_false_calls_refresh_assets(self, _target_platform, _gen, refresh):
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

    @patch("tasks.agent.refresh_assets")
    @patch("tasks.agent.generate_template")
    @patch("tasks.agent.get_target_platform", return_value="aix")
    def test_cross_aix_renders_aix_configs(self, _target_platform, gen, _refresh):
        ctx = MagicMock()

        generate_config_examples(
            ctx,
            flavor=AgentFlavor.base,
            skip_assets=True,
            build_tags=[],
            development=False,
            windows_sysprobe=False,
        )

        gen.assert_any_call(CORE_SCHEMA_FILE, "./cmd/agent/dist/datadog.yaml", "agent-py3", "aix")
        gen.assert_any_call(SYSPROBE_SCHEMA_FILE, "./cmd/agent/dist/system-probe.yaml", "system-probe", "aix")


class TestRefreshAssets(unittest.TestCase):
    @patch("tasks.agent.shutil")
    @patch("tasks.agent.os")
    @patch("tasks.agent.get_target_platform", return_value="win32")
    def test_cross_windows_skips_sysprobe_unless_requested(self, _target_platform, os_mod, shutil_mod):
        # Cross-compiling GOOS=windows from a non-Windows host: system-probe.yaml is never
        # generated (see generate_config_examples), so copying it here would crash with a
        # real shutil.copy. get_target_platform() must gate this the same way
        # generate_config_examples() does, not the host's sys.platform.
        os_mod.path.exists.return_value = False
        os_mod.path.join.side_effect = lambda *parts: "/".join(parts)

        refresh_assets(Context(), build_tags=[], development=False, windows_sysprobe=False)

        copied_paths = [call.args[0] for call in shutil_mod.copy.call_args_list]
        self.assertNotIn("./cmd/agent/dist/system-probe.yaml", copied_paths)

    @patch("tasks.agent.shutil")
    @patch("tasks.agent.os")
    @patch("tasks.agent.get_target_platform", return_value="win32")
    def test_cross_windows_copies_sysprobe_when_requested(self, _target_platform, os_mod, shutil_mod):
        os_mod.path.exists.return_value = False
        os_mod.path.join.side_effect = lambda *parts: "/".join(parts)

        refresh_assets(Context(), build_tags=[], development=False, windows_sysprobe=True)

        copied_paths = [call.args[0] for call in shutil_mod.copy.call_args_list]
        self.assertIn("./cmd/agent/dist/system-probe.yaml", copied_paths)


if __name__ == "__main__":
    unittest.main()
