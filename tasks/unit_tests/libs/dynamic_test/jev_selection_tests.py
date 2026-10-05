import json
import subprocess
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import MagicMock, call, patch

from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind
from tasks.libs.dynamic_test.jev_selection import (
    JevDynTestExecutor,
    NothingToEvaluateError,
    _candidates_for_job,
    _expand_variables,
    _suite_path,
    jev_selection,
)

MODULE = "tasks.libs.dynamic_test.jev_selection"
SHA = "a" * 40


class JevSelectionTests(unittest.TestCase):
    def test_nested_target(self):
        self.assertEqual(_suite_path("./tests/installer/unix/..."), "installer/unix")
        for target in ("../tests/fleet", "tests/../fleet", "$TARGETS", "pkg/foo"):
            with self.subTest(target=target), self.assertRaises(ValueError):
                _suite_path(target)

    def test_job_regexes(self):
        tests = {"TestFleetConfig", "TestFleetUpgrade", "TestFleetConfigExtra"}
        cases = [
            ('--run "TestFleetConfig$"', {"TestFleetConfig"}),
            ('--run "TestFleetConfig$" --skip "TestOther$"', {"TestFleetConfig"}),
            ('--run "TestFleet(Config|Upgrade)$"', {"TestFleetConfig", "TestFleetUpgrade"}),
            ('--run "TestFleetConfig$/Subtest" --skip "TestFleetConfig$/Other"', {"TestFleetConfig"}),
            ('--skip "TestFleetConfig$"', {"TestFleetUpgrade", "TestFleetConfigExtra"}),
            ('--run=TestFleetUpgrade --platform linux', {"TestFleetUpgrade"}),
            ("", tests),
        ]
        for params, expected in cases:
            with self.subTest(params=params):
                self.assertEqual(_candidates_for_job(params, tests), expected)

    def test_matrix_variable_expansion_preserves_regex_anchors(self):
        params = _expand_variables('--run "$E2E_MSI_TEST$"', {"E2E_MSI_TEST": "${TEST_NAME}", "TEST_NAME": "TestMSI"})
        self.assertEqual(params, '--run "TestMSI$"')
        self.assertEqual(_candidates_for_job(params, {"TestMSI", "TestMSIExtra"}), {"TestMSI"})
        with self.assertRaisesRegex(ValueError, "Unresolved CI variable"):
            _candidates_for_job('--run "$UNKNOWN$"', {"TestA"})
        with self.assertRaisesRegex(ValueError, "Cyclic"):
            _expand_variables("$A", {"A": "$B", "B": "$A"})

    @patch(f"{MODULE}.subprocess.run")
    def test_selector_uses_current_interpreter_and_module(self, run):
        def write_output(cmd, **kwargs):
            Path(cmd[cmd.index("--output") + 1]).write_text(json.dumps({"run": ["TestA"], "skip": []}))
            return SimpleNamespace(returncode=0)

        run.side_effect = write_output
        with patch.dict("os.environ", {"JEV_DC": "us1.ddbuild.io", "JEV_TOKEN_CMD": "custom-token"}):
            self.assertEqual(jev_selection("installer/unix")["run"], ["TestA"])
        cmd = run.call_args.args[0]
        self.assertEqual(cmd[:3], [__import__("sys").executable, "-m", "tasks.libs.dynamic_test.jev.jev_e2e_selector"])
        self.assertIn("installer/unix", cmd)
        self.assertIn("custom-token", cmd)

    @patch(f"{MODULE}.subprocess.run")
    def test_selector_failures_return_no_skip_decisions(self, run):
        for error in (OSError("missing interpreter"), subprocess.TimeoutExpired("selector", 1)):
            run.side_effect = error
            self.assertEqual(jev_selection("fleet"), {})
        run.side_effect = None
        run.return_value = SimpleNamespace(returncode=1, stderr="selector error")
        self.assertEqual(jev_selection("fleet"), {})

    @patch(f"{MODULE}.subprocess.run")
    def test_invalid_selector_summary(self, run):
        def write_output(cmd, **kwargs):
            Path(cmd[cmd.index("--output") + 1]).write_text('{"skip": "TestA"}')
            return SimpleNamespace(returncode=0)

        run.side_effect = write_output
        self.assertEqual(jev_selection("fleet"), {})

    @patch(f"{MODULE}.list_suites")
    @patch(f"{MODULE}.jev_selection")
    def test_fail_open_and_cached_predictions(self, select, discover):
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor._index = DynamicTestIndex({"job": {"jev": ["TestA", "TestB", "TestUnknown"]}})
        executor._suites = {"fleet"}
        discover.return_value = [(name, "path", "code") for name in ("TestA", "TestB", "TestUnknown")]
        # A missing decision is run, and contradictory decisions run as well.
        select.return_value = {"run": ["TestB"], "skip": ["TestA", "TestB"]}
        self.assertEqual(executor.tests_to_run_per_job([]), {"job": {"TestB", "TestUnknown"}})
        self.assertEqual(executor.tests_to_skip("job", []), {"TestA"})
        self.assertEqual(executor.tests_to_run("job", []), {"TestB", "TestUnknown"})
        select.assert_called_once_with("fleet")
        executor._jev_run_tests = None
        select.return_value = {}
        self.assertEqual(executor.tests_to_skip("job", []), set())
        self.assertEqual(executor.tests_to_run("unknown-job", []), set())

    @patch(f"{MODULE}.list_suites", return_value=[("TestDuplicate", "path", "code")])
    @patch(f"{MODULE}.jev_selection")
    def test_duplicate_names_run_if_either_suite_runs(self, select, _):
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor._index = DynamicTestIndex({"job": {"jev": ["TestDuplicate"]}})
        executor._suites = {"a", "b"}
        select.side_effect = [{"run": [], "skip": ["TestDuplicate"]}, {}]
        self.assertEqual(executor.tests_to_run("job", []), {"TestDuplicate"})

    @patch(f"{MODULE}.Path.is_dir", return_value=True)
    @patch(f"{MODULE}.list_suites")
    @patch(f"{MODULE}.resolve_gitlab_ci_configuration")
    @patch(f"{MODULE}.get_pipeline")
    def test_pipeline_scoping_pagination_and_real_index(self, get_pipeline, resolve, discover, _):
        pipeline = get_pipeline.return_value
        pipeline.sha = SHA
        pipeline.jobs.list.side_effect = [
            iter(
                [
                    SimpleNamespace(name='new-e2e-fleet: [--run "TestFleet$"]', id=1, allow_failure=False),
                    SimpleNamespace(name="new-e2e-unit-tests", id=3, allow_failure=False),
                ]
            ),
            iter(
                [
                    SimpleNamespace(name="new-e2e-installer", id=2, allow_failure=True),
                    SimpleNamespace(name="unit-tests", id=4, allow_failure=False),
                ]
            ),
        ]
        resolve.return_value = {
            "new-e2e-fleet": {
                "variables": {"TARGETS": "./tests/fleet"},
                "parallel": {"matrix": [{"EXTRA_PARAMS": ['--run "TestFleet$"']}]},
            },
            "new-e2e-installer": {"variables": {"TARGETS": "./tests/installer/unix"}},
            "new-e2e-unit-tests": {"script": "unit tests"},
        }
        discover.side_effect = lambda directory: [
            (name, "path", "code")
            for name in (["TestFleet", "TestFleetExtra"] if directory.endswith("fleet") else ["TestInstaller"])
        ]
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        executor.init_index()
        get_pipeline.assert_called_once_with("DataDog/datadog-agent", "42")
        self.assertEqual(
            pipeline.jobs.list.call_args_list,
            [call(scope="success", iterator=True), call(scope="failed", iterator=True)],
        )
        self.assertIsInstance(executor.index(), DynamicTestIndex)
        self.assertEqual(executor.kind, IndexKind.JEV)
        self.assertEqual(
            executor.index().get_indexed_tests_for_job('new-e2e-fleet: [--run "TestFleet$"]'), {"TestFleet"}
        )
        self.assertEqual(executor.index().get_indexed_tests_for_job("new-e2e-installer"), {"TestInstaller"})
        self.assertEqual(executor.unreliable_jobs, {"new-e2e-installer"})
        self.assertEqual(executor.job_ids["new-e2e-installer"], "2")
        self.assertEqual(executor._suites, {"fleet", "installer/unix"})

    @patch(f"{MODULE}.get_pipeline")
    def test_rejects_wrong_commit_and_empty_pipeline(self, get_pipeline):
        pipeline = get_pipeline.return_value
        pipeline.sha = "b" * 40
        executor = JevDynTestExecutor(MagicMock(), SHA, "42")
        with self.assertRaisesRegex(RuntimeError, "SHA"):
            executor.init_index()
        pipeline.jobs.list.assert_not_called()
        pipeline.sha = SHA
        pipeline.jobs.list.side_effect = [iter([]), iter([])]
        with self.assertRaisesRegex(NothingToEvaluateError, "No completed E2E jobs"):
            executor.init_index()
        # Completed new-e2e jobs, but none is an E2E test run (no TARGETS,
        # e.g. cleanup/unit-test jobs): benign as well, not a failure
        pipeline.jobs.list.side_effect = [
            iter([SimpleNamespace(name="new-e2e-unit-tests", id=1, allow_failure=False)]),
            iter([]),
        ]
        config = {"new-e2e-unit-tests": {"variables": {}}}
        with (
            patch(f"{MODULE}.resolve_gitlab_ci_configuration", return_value=config),
            self.assertRaisesRegex(NothingToEvaluateError, "No completed E2E test jobs"),
        ):
            executor.init_index()
