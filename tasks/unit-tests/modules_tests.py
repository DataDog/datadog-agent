import json
import os
import subprocess
import unittest
from typing import Any, Dict, Set

from tasks.modules import AGENT_MODULE_PATH_PREFIX, DEFAULT_MODULES

"""
Here is an abstract of the go.mod file format:

{
    "Module": {"Path": "github.com/DataDog/datadog-agent"},
    "Go": "1.21",
    "Require": [
        {"Path": "github.com/DataDog/datadog-agent/pkg/config/logs", "Version": "v0.51.0-rc.2"},
        {"Path": "k8s.io/kms", "Version": "v0.27.6", "Indirect": true},
    ],
    "Exclude": [
        {"Path": "github.com/knadh/koanf/maps", "Version": "v0.1.1"},
        {"Path": "github.com/knadh/koanf/providers/confmap", "Version": "v0.1.0"},
    ],
    "Replace": [
        {
            "Old": {"Path": "github.com/cihub/seelog"},
            "New": {"Path": "github.com/cihub/seelog", "Version": "v0.0.0-20151216151435-d2c6e5aa9fbf"},
        },
        {
            "Old": {"Path": "github.com/DataDog/datadog-agent/cmd/agent/common/path"},
            "New": {"Path": "./cmd/agent/common/path/"},
        },
    ],
    "Retract": [{"Low": "v0.9.0", "High": "v0.9.0"}, {"Low": "v0.8.0", "High": "v0.8.0"}],
}
"""


class TestModules(unittest.TestCase):
    def load_go_mod(self, module_path: str) -> Any:
        """Loads the go.mod file as a JSON object"""
        go_mod_path = os.path.join(module_path, "go.mod")
        res = subprocess.run(["go", "mod", "edit", "-json", go_mod_path], capture_output=True)
        self.assertEqual(res.returncode, 0)

        return json.loads(res.stdout)

    def get_agent_required(self, module: Dict) -> Set[str]:
        """Returns the set of required datadog-agent modules"""
        if "Require" not in module:
            return set()

        required = module["Require"]
        if required is None:
            return set()

        results = set()
        self.assertIsInstance(required, list)
        for req in required:
            self.assertIsInstance(req, dict)
            self.assertIn("Path", req)
            path = req["Path"]

            self.assertIsInstance(path, str)
            if path.startswith(AGENT_MODULE_PATH_PREFIX):
                results.add(path)

        return results

    def get_agent_replaced(self, module: Dict) -> Set[str]:
        """Returns the set of replaced datadog-agent modules"""
        if "Replace" not in module:
            return set()

        replaced = module["Replace"]
        if replaced is None:
            return set()

        results = set()
        self.assertIsInstance(replaced, list)
        for req in replaced:
            self.assertIsInstance(req, dict)
            self.assertIn("Old", req)
            old = req["Old"]

            self.assertIsInstance(old, dict)
            self.assertIn("Path", old)
            oldpath = old["Path"]
            if oldpath.startswith(AGENT_MODULE_PATH_PREFIX):
                results.add(oldpath)

        return results

    def test_modules_replace_agent(self):
        """Ensure that all required datadog-agent modules are replaced"""
        for module_path in DEFAULT_MODULES.keys():
            with self.subTest(module_path=module_path):
                module = self.load_go_mod(module_path)
                self.assertIsInstance(module, dict)
                required = self.get_agent_required(module)
                replaced = self.get_agent_replaced(module)
                required_not_replaced = required - replaced
                self.assertEqual(required_not_replaced, set(), f"in module {module_path}")
