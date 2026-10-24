import importlib
import sys
import unittest
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock, patch


def click_option(*_args, **_kwargs):
    return lambda function: function


LAB_PYTHONPATH = Path(__file__).parents[2] / ".dda" / "extend" / "pythonpath"
sys.path.insert(0, str(LAB_PYTHONPATH))

click_module = ModuleType("click")
click_module.__dict__["option"] = click_option
with patch.dict(sys.modules, {"click": click_module}):
    kind_provider = importlib.import_module("lab.providers.local.kind")


class TestKindOptions(unittest.TestCase):
    def test_no_agent_wins_over_load_image(self):
        options = kind_provider.KindOptions(name="dev", no_agent=True, load_image="myagent:dev")

        self.assertFalse(options.wants_agent)


class TestKindProvider(unittest.TestCase):
    @patch("lab.kind.show_cluster_info")
    @patch("lab.kind.load_image")
    @patch("lab.kind.create_cluster")
    @patch("lab.kind.cluster_exists", return_value=False)
    def test_no_agent_loads_image_without_installing(
        self, _cluster_exists, _create_cluster, load_image, _show_cluster_info
    ):
        app = MagicMock()
        provider = kind_provider.KindProvider()
        options = kind_provider.KindOptions(name="dev", no_agent=True, load_image="myagent:dev")

        with patch.object(provider, "_install_agent") as install_agent:
            metadata = provider.create(app, options)

        load_image.assert_called_once_with(app, "dev", "myagent:dev")
        install_agent.assert_not_called()
        self.assertFalse(metadata["agent_installed"])
