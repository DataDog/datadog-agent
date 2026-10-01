"""Focused offline tests for SMP policy, links, conversion, and the CLI."""

from __future__ import annotations

import copy
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

import convert_old_report_to_v1 as converter
import smp_ci

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = Path(smp_ci.__file__).resolve()


def bounds_check(passed: int = 1, total: int = 1) -> dict:
    return {
        "name": "memory_usage",
        "series": "total_pss_bytes",
        "unit": "bytes",
        "lower_bound": None,
        "upper_bound": 100,
        "pass_count": passed,
        "total_count": total,
        "min_observed": 10 if total else None,
        "max_observed": 20 if total else None,
    }


def experiment(name: str = "quality_gate_test", passed: int = 1, total: int = 1, erratic: bool = False) -> dict:
    return {
        "name": name,
        "erratic": erratic,
        "optimization_goal": None,
        "bounds_checks": [bounds_check(passed, total)],
        "quantile_checks": [],
    }


def report(*experiments: dict) -> dict:
    return {
        "$schema": converter.SCHEMA_ID,
        "job": {
            "id": "00000000-0000-0000-0000-000000000000",
            "baseline_sha": "a" * 40,
            "comparison_sha": "b" * 40,
            "metrics_query_start": 1800000000,
            "metrics_query_end": 1800000060,
            "tolerances": {"p_value": 0.05, "effect_size": 0.05, "coefficient_of_variation_limit": 0.2},
        },
        "experiments": list(experiments),
        "analysis_errors": [],
        "failed_replicates": [],
    }


def optimization_goal(significant: bool = True, improvement: bool = False, runtime_erratic: bool = False) -> dict:
    return {
        "goal": "memory",
        "trial_count": 1,
        "max_trial_count": 1,
        "result": {
            "percent_change": 10,
            "confidence_interval": {"lower": 9, "upper": 11},
            "confidence_percent": 95,
            "is_significant_change": significant,
            "is_improvement": improvement,
            "is_erratic": runtime_erratic,
        },
    }


class DecisionTests(unittest.TestCase):
    def test_bounds_and_gate_scenarios(self) -> None:
        cases = [
            ("pass", experiment(), "passed", "passed"),
            ("failed gate", experiment(passed=0), "failed", "failed"),
            ("empty gate", experiment(passed=0, total=0), "passed", "failed"),
            ("non-gated bounds failure", experiment(name="other", passed=0), "failed", "passed"),
            ("erratic gate", experiment(passed=0, erratic=True), "passed", "failed"),
            ("erratic non-gate", experiment(name="other", passed=0, erratic=True), "passed", "passed"),
        ]
        for name, xp, bounds, job in cases:
            with self.subTest(name=name):
                result = smp_ci.render_decision(report(xp))
                self.assertEqual(result.decision, {"regressions": "passed", "all_bounds_checks": bounds, "job": job})
                self.assertIn("CI Pass/Fail Decision", result.tail_section)
                self.assertIn("**Failed.**" if job == "failed" else "**Passed.**", result.tail_section)

    def test_regression_scenarios(self) -> None:
        cases = [
            ("regression", optimization_goal(), False, "failed"),
            ("improvement", optimization_goal(improvement=True), False, "passed"),
            ("not significant", optimization_goal(significant=False), False, "passed"),
            ("configured erratic", optimization_goal(), True, "passed"),
            ("runtime erratic", optimization_goal(runtime_erratic=True), False, "failed"),
            ("missing result", {"trial_count": 1, "max_trial_count": 1, "result": None}, False, "failed"),
            ("omitted result", {"trial_count": 1, "max_trial_count": 1}, False, "failed"),
            ("missing erratic analysis", {"trial_count": 1, "max_trial_count": 1}, True, "failed"),
        ]
        for name, goal, erratic, expected in cases:
            with self.subTest(name=name):
                xp = experiment(erratic=erratic)
                xp["optimization_goal"] = goal
                result = smp_ci.render_decision(report(xp))
                self.assertEqual(result.decision["regressions"], expected)
                self.assertEqual(result.decision["job"], "passed")

    def test_no_gates_and_optional_goal(self) -> None:
        xp = experiment(name="other")
        del xp["optimization_goal"]
        for fixture in [report(), report(xp)]:
            with self.subTest(experiments=fixture["experiments"]):
                result = smp_ci.render_decision(fixture)
                self.assertEqual(set(result.decision.values()), {"passed"})
                self.assertIn("no quality gates", result.tail_section)

    def test_empty_gate_has_a_missing_data_explanation(self) -> None:
        result = smp_ci.render_decision(report(experiment(passed=0, total=0)))
        self.assertIn("No comparison replicate data. Gate **FAILED**.", result.tail_section)
        self.assertNotIn("Failed 0 which is > 0", result.tail_section)

    def test_decision_does_not_mutate_input(self) -> None:
        fixture = report(experiment(passed=0))
        before = copy.deepcopy(fixture)
        smp_ci.render_decision(fixture)
        self.assertEqual(fixture, before)


class LinkTests(unittest.TestCase):
    def test_agent_links_for_all_experiments(self) -> None:
        fixture = report(experiment(), experiment(name="python_openmetrics"), experiment(name="tcp_rr"))
        extra = smp_ci.build_extra(fixture, "CI decision")
        self.assertEqual(extra["tail_section"], "CI decision")
        self.assertEqual(extra["links"]["header"], ("metrics_dashboard", "profiles"))
        self.assertEqual(extra["links"]["failed_replicates"], ("debug_dashboard",))
        for xp in fixture["experiments"]:
            links = extra["links"]["experiments"][xp["name"]]
            dashboard = () if xp["name"] == "tcp_rr" else ("bounds_dashboard",)
            self.assertEqual(links["default"], ("logs",) + dashboard)
            self.assertEqual(links["optimization_goal"], ("logs",) + dashboard)
            self.assertEqual(links["checks"]["memory_usage"], dashboard)

    def test_quantile_only_check_gets_a_link_entry(self) -> None:
        xp = experiment()
        xp["quantile_checks"] = [
            {
                "bounds_check_name": "latency",
                "series": "latency",
                "unit": "none",
                "quantile": 0.99,
                "value": 10,
                "analysis_method": "harrell_davis",
            }
        ]
        extra = smp_ci.build_extra(report(xp), "tail")
        self.assertEqual(extra["links"]["experiments"][xp["name"]]["checks"]["latency"], ("bounds_dashboard",))


class ConversionTests(unittest.TestCase):
    def test_empty_comparison_is_preserved_and_gates_the_job(self) -> None:
        for results in [{}, {"comparison": []}, {"baseline": [{"passed": True}]}]:
            with self.subTest(results=results):
                check = converter.build_bounds_check(
                    "memory_usage", {"bounds_check_type": "bounds", "results": results}
                )
                self.assertIsNotNone(check)
                self.assertEqual(check["total_count"], 0)
                self.assertEqual(check["pass_count"], 0)
                self.assertIsNone(check["min_observed"])
                self.assertIsNone(check["max_observed"])
                xp = experiment()
                xp["bounds_checks"] = [check]
                self.assertEqual(smp_ci.render_decision(report(xp)).decision["job"], "failed")

    def test_only_comparison_replicates_count(self) -> None:
        raw = {
            "bounds_check_type": "bounds",
            "results": {
                "baseline": [{"passed": False, "min_observed": 500, "max_observed": 600}],
                "comparison": [{"passed": True, "min_observed": 10, "max_observed": 20}],
            },
        }
        check = converter.build_bounds_check("memory_usage", raw)
        self.assertEqual((check["pass_count"], check["total_count"]), (1, 1))
        self.assertEqual((check["min_observed"], check["max_observed"]), (10, 20))

    def test_malformed_legacy_json_fails(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "report.json"
            path.write_text("{invalid", encoding="utf-8")
            result = subprocess.run(
                [sys.executable, converter.__file__, str(path)], capture_output=True, text=True, check=False
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(path.with_name("report.v1.json").exists())


class CliTests(unittest.TestCase):
    def setUp(self) -> None:
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.smp = self.root / "smp"
        markdown_prefix = '# Report\nquote: " backslash: \\ Unicode: \u00e9\n'
        self.smp.write_text(
            f"#!{sys.executable}\n"
            "import json, pathlib, sys\n"
            "assert sys.argv[1:3] == ['report', 'render']\n"
            "extra = json.loads(pathlib.Path(sys.argv[sys.argv.index('--extra') + 1]).read_text())\n"
            "assert extra['links']['header'] == ['metrics_dashboard', 'profiles']\n"
            f"sys.stdout.write({markdown_prefix!r} + extra['tail_section'] + '\\n')\n",
            encoding="utf-8",
        )
        self.smp.chmod(0o700)

    def run_cli(self, fixture: dict, payload: str = "payload.json") -> subprocess.CompletedProcess:
        (self.root / "report.v1.json").write_text(json.dumps(fixture), encoding="utf-8")
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--smp",
                "./smp",
                "--report",
                "report.v1.json",
                "--commit",
                "test-commit",
                "--template",
                str(ROOT / "test/regression/report_template.md.j2"),
                "--output-report-md",
                "report.md",
                "--output-decision",
                "decision.json",
                "--output-pr-comment",
                payload,
                "--output-failure-mark",
                "smp-failed-flag",
            ],
            cwd=self.root,
            capture_output=True,
            text=True,
            check=False,
        )

    def test_pass_and_policy_failure_both_return_zero(self) -> None:
        for passed in [1, 0]:
            with self.subTest(passed=passed):
                result = self.run_cli(report(experiment(passed=passed)))
                self.assertEqual(result.returncode, 0, result.stderr)
                decision = json.loads((self.root / "decision.json").read_text())
                self.assertEqual((self.root / "smp-failed-flag").exists(), decision["job"] == "failed")
                if passed == 0:
                    self.assertEqual((self.root / "smp-failed-flag").read_bytes(), b"")
                payload = json.loads((self.root / "payload.json").read_text())
                markdown = (self.root / "report.md").read_text(encoding="utf-8")
                self.assertEqual(
                    payload,
                    {
                        "org": "DataDog",
                        "repo": "datadog-agent",
                        "commit": "test-commit",
                        "header": "Regression Detector",
                        "message": markdown,
                    },
                )
                self.assertIn('quote: " backslash: \\ Unicode: \u00e9\n', markdown)
                self.assertIn("CI Pass/Fail Decision", markdown)

    def test_passing_rerun_removes_failure_marker(self) -> None:
        self.assertEqual(self.run_cli(report(experiment(passed=0))).returncode, 0)
        self.assertTrue((self.root / "smp-failed-flag").exists())
        self.assertEqual(self.run_cli(report(experiment())).returncode, 0)
        self.assertFalse((self.root / "smp-failed-flag").exists())

    def test_render_failure_returns_nonzero_and_preserves_diagnostic(self) -> None:
        self.smp.write_text(f"#!{sys.executable}\nimport sys\nsys.stderr.write('render failed\\n')\nsys.exit(17)\n")
        result = self.run_cli(report(experiment()))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("render failed", result.stderr)
        self.assertFalse((self.root / "payload.json").exists())
        self.assertFalse((self.root / "decision.json").exists())

    def test_publication_failure_returns_nonzero(self) -> None:
        (self.root / "payload_directory").mkdir()
        result = self.run_cli(report(experiment()), payload="payload_directory")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("error:", result.stderr)

    def test_malformed_v1_json_returns_nonzero(self) -> None:
        args = [
            "--smp",
            str(self.smp),
            "--report",
            str(self.root / "report.v1.json"),
            "--commit",
            "test",
            "--template",
            "unused",
            "--output-report-md",
            str(self.root / "report.md"),
            "--output-decision",
            str(self.root / "decision.json"),
            "--output-pr-comment",
            str(self.root / "payload.json"),
            "--output-failure-mark",
            str(self.root / "smp-failed-flag"),
        ]
        (self.root / "report.v1.json").write_text("{invalid", encoding="utf-8")
        result = subprocess.run([sys.executable, str(SCRIPT), *args], capture_output=True, text=True, check=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "payload.json").exists())


if __name__ == "__main__":
    unittest.main()
