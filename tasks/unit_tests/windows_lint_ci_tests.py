"""Check that inherited installer settings reach every Windows CI gate."""

import unittest
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]


class TestWindowsLintCI(unittest.TestCase):
    def test_root_editorconfig_enables_windows_workflow_include_and_lint_job(self):
        config = yaml.load((REPO / ".gitlab-ci.yml").read_text("utf-8"), Loader=yaml.BaseLoader)
        job = yaml.load((REPO / ".gitlab/windows/build/lint/windows.yml").read_text("utf-8"), Loader=yaml.BaseLoader)
        windows = next(include for include in config["include"] if include.get("local") == ".gitlab/windows/**.yml")
        self.assertTrue(
            any(
                ".gitlab/windows/build/lint/windows.yml" in rule.get("changes", {}).get("paths", [])
                for rule in config[".on_invoke_tasks_changes"]
                if isinstance(rule, dict)
            )
        )
        for name, rules in [
            ("workflow", config["workflow"]["rules"]),
            ("include", windows["rules"]),
            ("lint job", job["lint_windows-x64"]["rules"]),
        ]:
            with self.subTest(gate=name):
                matching = next(
                    rule
                    for rule in rules
                    if isinstance(rule, dict) and ".editorconfig" in rule.get("changes", {}).get("paths", [])
                )
                self.assertNotEqual(matching.get("when", "on_success"), "never")
                if name == "workflow":
                    self.assertEqual(matching["variables"]["SKIP_WINDOWS"], "false")


if __name__ == "__main__":
    unittest.main()
