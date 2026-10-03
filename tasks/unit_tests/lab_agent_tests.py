import importlib.util
import unittest
from pathlib import Path


AGENT_MODULE_PATH = Path(__file__).parents[2] / ".dda" / "extend" / "pythonpath" / "lab" / "agent.py"
AGENT_MODULE_SPEC = importlib.util.spec_from_file_location("lab.agent", AGENT_MODULE_PATH)
assert AGENT_MODULE_SPEC is not None
assert AGENT_MODULE_SPEC.loader is not None
AGENT_MODULE = importlib.util.module_from_spec(AGENT_MODULE_SPEC)
AGENT_MODULE_SPEC.loader.exec_module(AGENT_MODULE)


class TestParseImage(unittest.TestCase):
    def test_parse_image(self):
        test_cases = (
            ("datadog/agent", ("datadog/agent", "latest")),
            ("datadog/agent:dev", ("datadog/agent", "dev")),
            ("localhost:5000/datadog/agent", ("localhost:5000/datadog/agent", "latest")),
            ("localhost:5000/datadog/agent:dev", ("localhost:5000/datadog/agent", "dev")),
        )

        for image, expected in test_cases:
            with self.subTest(image=image):
                self.assertEqual(AGENT_MODULE._parse_image(image), expected)
