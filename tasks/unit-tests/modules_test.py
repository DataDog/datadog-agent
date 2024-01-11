import json
import os
import subprocess
import unittest
from typing import Any, Set

from ..modules import DEFAULT_MODULES

AGENT_MODULE_PATH_PREFIX = "github.com/DataDog/datadog-agent/"


class TestModules(unittest.TestCase):
    def load_go_mod(self, module_path: str) -> Any:
        """Loads the go.mod file as a JSON object"""
        go_mod_path = os.path.join(module_path, "go.mod")
        res = subprocess.run(["go", "mod", "edit", "-json", go_mod_path], capture_output=True)
        self.assertEqual(res.returncode, 0)

        return json.loads(res.stdout)

    def get_agent_required(self, module: Any) -> Set[str]:
        """Returns the set of required datadog-agent modules"""
        self.assertIsInstance(module, dict)
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

    def get_agent_replaced(self, module: Any) -> Set[str]:
        """Returns the set of replaced datadog-agent modules"""
        self.assertIsInstance(module, dict)
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
                required = self.get_agent_required(module)
                replaced = self.get_agent_replaced(module)
                required_not_replaced = required - replaced
                self.assertEqual(required_not_replaced, set(), f"in module {module_path}")


if __name__ == '__main__':
    unittest.main()
