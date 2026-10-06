import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import ANY, patch

from tasks.libs.dynamic_test.jev.jev_client import decide, get_ai_gateway_token
from tasks.libs.dynamic_test.jev.jev_e2e_selector import select_suite
from tasks.libs.dynamic_test.jev.pr_context import fetch_pr_info
from tasks.libs.dynamic_test.jev.test_discovery import list_suites


def answers(should=0.5, relation="code_under_test", confidence=0.8):
    return {"should_execute": {"noul": should}, "relation": {"choice": relation}, "confidence": {"score": confidence}}


class JevToolsTests(unittest.TestCase):
    def test_decision(self):
        self.assertEqual(decide(answers())["decision"], "run")
        self.assertEqual(decide(answers(should=0.01))["decision"], "skip")
        self.assertEqual(decide(answers(relation="unrelated"))["decision"], "skip")
        self.assertEqual(decide(answers(should=0.1))["decision"], "run")

    def test_invalid_model_scores_and_relations(self):
        for value in (float("nan"), float("inf"), None, True, "0.1", -1, 2):
            with self.subTest(value=value), self.assertRaises(ValueError):
                decide(answers(should=value))
        with self.assertRaises(ValueError):
            decide(answers(relation="typo"))
        with self.assertRaises(KeyError):
            decide({})

    @patch("tasks.libs.dynamic_test.jev.jev_client.datadog_infra_token")
    def test_auth_uses_the_repo_infra_token_helper(self, infra_token):
        """The fallback delegates to datadog_infra_token (authanywhere in CI, ddtool locally)."""
        infra_token.return_value = "Bearer token"
        with patch.dict("os.environ", {}, clear=True):
            self.assertEqual(get_ai_gateway_token(dc="us1.ddbuild.io"), "token")
            infra_token.assert_called_once_with(ANY, "rapid-ai-platform", "us1.ddbuild.io")
            # The explicit token/token_cmd overrides short-circuit before it
            self.assertEqual(get_ai_gateway_token(token="override"), "override")
            infra_token.assert_called_once_with(ANY, "rapid-ai-platform", "us1.ddbuild.io")

    @patch("tasks.libs.dynamic_test.jev.jev_client.Context")
    def test_token_cmd_runs_through_the_shell_as_is(self, context):
        context.return_value.run.return_value.stdout = "token\n"
        with patch.dict("os.environ", {}, clear=True):
            self.assertEqual(get_ai_gateway_token(token_cmd='tool --name "two words"'), "token")
        # The token command is a shell command string: passed as-is, no
        # argv round-trip
        context.return_value.run.assert_called_once_with(
            'tool --name "two words"', hide=True, encoding="utf-8", timeout=60
        )

    @patch("tasks.libs.dynamic_test.jev.pr_context.GithubAPI")
    @patch("tasks.libs.dynamic_test.jev.pr_context.git", return_value="feature/nested")
    def test_pr_lookup_by_branch_uses_the_shared_github_api(self, _, github_api):
        github_api.return_value.get_pr_for_branch.return_value = iter(
            [SimpleNamespace(number=12, title="Title", body="Description")]
        )
        with patch.dict("os.environ", {"GITHUB_TOKEN": "fake"}, clear=True):
            info = fetch_pr_info("main", None)
        self.assertEqual(info["number"], 12)
        self.assertEqual(info["title"], "Title")
        self.assertEqual(info["description"], "Description")
        github_api.return_value.get_pr_for_branch.assert_called_once_with(head_branch_name="feature/nested")

    @patch("tasks.libs.dynamic_test.jev.pr_context.GithubAPI")
    @patch("tasks.libs.dynamic_test.jev.pr_context.git", return_value="feature/nested")
    def test_pr_lookup_without_token_or_without_pr_degrades_gracefully(self, _, github_api):
        with patch.dict("os.environ", {}, clear=True):
            info = fetch_pr_info("main", None)
        self.assertEqual(info, {"branch": "feature/nested", "title": "", "description": ""})
        github_api.assert_not_called()
        github_api.return_value.get_pr_for_branch.return_value = iter([])
        with patch.dict("os.environ", {"GITHUB_TOKEN": "fake"}):
            info = fetch_pr_info("main", None)
        self.assertNotIn("number", info)
        self.assertEqual(info["title"], "")

    def test_discovery_ignores_helpers(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "helpers.go").write_text("package test\nfunc TestHelper(t *testing.T) {}\n")
            Path(directory, "suite_test.go").write_text(
                "package test\nfunc TestSuite(t *testing.T) {}\nfunc TestHelper(x string) {}\n"
                "func (s *Suite) TestMethod() {}\n"
            )
            self.assertEqual([entry[0] for entry in list_suites(directory)], ["TestSuite"])

    def test_selector_fails_open_on_malformed_response_and_duplicate_names(self):
        module = "tasks.libs.dynamic_test.jev.jev_e2e_selector"
        with tempfile.TemporaryDirectory() as directory:
            output = str(Path(directory, "decisions.json"))
            with (
                patch(f"{module}.os.path.isdir", return_value=True),
                patch(f"{module}.fetch_ddci_metadata", return_value=None),
                patch(f"{module}.fetch_pr_info", return_value={}),
                patch(f"{module}.changed_files", return_value=([], "base")),
                patch(f"{module}.pr_diff", return_value=""),
                patch(f"{module}.suite_definition", return_value=("", "")),
                patch(f"{module}.get_ai_gateway_token", return_value="fake"),
                patch(f"{module}.gitlab_section"),
                patch(
                    f"{module}.list_suites",
                    return_value=[
                        ("TestDuplicate", "one_test.go", "code"),
                        ("TestDuplicate", "two_test.go", "code"),
                        ("TestUnavailable", "three_test.go", "code"),
                    ],
                ),
                patch(
                    f"{module}.ask_jev",
                    side_effect=[{"answers": answers(0.0, "unrelated")}, {"answers": {}}, RuntimeError("offline")],
                ),
            ):
                select_suite("fleet", workers=1, output=output)
            summary = json.loads(Path(output).read_text())
            self.assertEqual(summary["run"], ["TestDuplicate", "TestUnavailable"])
            self.assertEqual(summary["skip"], [])
            self.assertIn("error", summary["decisions"][1])
