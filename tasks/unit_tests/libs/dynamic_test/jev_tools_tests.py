import argparse
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tasks.libs.dynamic_test.jev.jev_client import decide, get_ai_gateway_token
from tasks.libs.dynamic_test.jev.jev_e2e_selector import main
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

    @patch("tasks.libs.dynamic_test.jev.jev_client.run_cmd", return_value="token")
    def test_auth_uses_matching_datacenter_and_quoted_commands(self, run):
        args = argparse.Namespace(token=None, token_cmd=None, dc="us1.ddbuild.io")
        with patch.dict("os.environ", {}, clear=True):
            self.assertEqual(get_ai_gateway_token(args), "token")
            run.assert_called_once_with(
                ["authanywhere", "--audience", "rapid-ai-platform", "--raw", "--dc", "us1.ddbuild.io"]
            )
            args.token_cmd = 'tool --name "two words"'
            get_ai_gateway_token(args)
            run.assert_called_with(["tool", "--name", "two words"])
            args.token = "override"
            self.assertEqual(get_ai_gateway_token(args), "override")

    @patch("tasks.libs.dynamic_test.jev.pr_context.current_branch", return_value="feature/nested")
    @patch("tasks.libs.dynamic_test.jev.pr_context.urllib.request.urlopen")
    def test_github_head_uses_owner_not_repository(self, urlopen, _):
        urlopen.return_value.__enter__.return_value.read.return_value = json.dumps(
            [{"number": 12, "title": "Title", "body": "Description"}]
        ).encode()
        with patch.dict("os.environ", {"GITHUB_TOKEN": "fake"}):
            self.assertEqual(fetch_pr_info("main", None)["number"], 12)
        self.assertIn("head=DataDog:feature%2Fnested", urlopen.call_args.args[0].full_url)

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
                patch("sys.argv", ["selector", "--suite", "fleet", "--output", output, "--workers", "1"]),
                patch(f"{module}.os.path.isdir", return_value=True),
                patch(f"{module}.fetch_ddci_metadata", return_value=None),
                patch(f"{module}.fetch_pr_info", return_value={}),
                patch(f"{module}.changed_files", return_value=([], "base")),
                patch(f"{module}.pr_diff", return_value=""),
                patch(f"{module}.suite_definition", return_value=("", "")),
                patch(f"{module}.get_ai_gateway_token", return_value="fake"),
                patch(f"{module}.print_collapsible"),
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
                self.assertEqual(main(), 0)
            summary = json.loads(Path(output).read_text())
            self.assertEqual(summary["run"], ["TestDuplicate", "TestUnavailable"])
            self.assertEqual(summary["skip"], [])
            self.assertIn("error", summary["decisions"][1])
