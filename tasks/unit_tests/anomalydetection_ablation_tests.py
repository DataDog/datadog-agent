import importlib.util
import io
import json
import sqlite3
import tempfile
import threading
import unittest
import zipfile
from contextlib import chdir
from copy import deepcopy
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from unittest.mock import MagicMock, Mock, patch

from tasks.anomalydetection import eval_pipeline, publish_ddeval_testbench
from tasks.libs.anomalydetection.ablation import (
    AgentCIClient,
    RemoteStudy,
    TransientError,
    metric,
    read_json,
)
from tasks.libs.anomalydetection.ablation_ci import restore_checkpoint


def spec(**overrides):
    return {
        "dataset": "Golden 25",
        "dataset_version": 0,
        "dataset_filter": {"limit": 1},
        "jobs": 6,
        "seed": 42,
        "source_commit": "test-commit",
        "experiment_config": {
            "executor_config": {"binary_artifacts": {"testbench": {"uri": "s3://bucket/binary", "sha256": "a" * 64}}},
            "input_parameters": {"testbench_config": {"logs_only": True, "components": {"bocpd": {"custom": 12}}}},
        },
        "n_combos": 1,
        "n_trials_search": 1,
        "n_trials_tune": 1,
        "m_runs": 1,
        "force_enable": [],
        "force_disable": [],
        **overrides,
    }


def completed(**overrides):
    return {
        "status": "completed",
        "completed": True,
        "dataset_version": 7,
        "metrics_json": '{"f1": 0.75, "precision": 0.8, "recall": 0.7}',
        "experiment_url": "https://app.datadoghq.com/llm/experiments/test",
        **overrides,
    }


class TestRemoteStudy(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.output = Path(temporary.name)
        self.client = Mock()
        self.requests = []

        def post(attributes, *, result=False, **_):
            if result:
                return completed()
            self.requests.append(attributes)
            return {"id": attributes["workflow_id"], "run_id": "run-1"}

        self.client.post.side_effect = post

    def test_latest_version_pinned_and_base_config_retained(self):
        config = {"components": {"bocpd": {"enabled": True}}}
        with RemoteStudy(self.output, spec(), self.client) as remote:
            remote.evaluate("trial_000", config)
            remote.evaluate("trial_001", config)
        self.assertEqual([r["dataset_version"] for r in self.requests], [0, 7])
        self.assertEqual(self.requests[0]["scenario_concurrency"], 6)
        experiment = json.loads(self.requests[0]["experiment_config"])
        base = experiment["input_parameters"]["testbench_config"]
        self.assertTrue(base["logs_only"])
        self.assertEqual(base["components"]["bocpd"], {"custom": 12, "enabled": True})

    def test_resume_recovers_lost_start_response_without_resubmission(self):
        self.client.post.side_effect = [TransientError("lost start response"), KeyboardInterrupt()]
        with self.assertRaises(KeyboardInterrupt), RemoteStudy(self.output, spec(), self.client) as remote:
            remote.evaluate("trial", {})
        saved = read_json(self.output / "study.json")["trials"]["trial"]
        self.client.post.reset_mock(side_effect=True)
        self.client.post.return_value = completed()
        with RemoteStudy(self.output, spec(), self.client, resume=True) as remote:
            self.assertEqual(remote.evaluate("trial", {}), 0.75)
        self.client.post.assert_called_once()
        self.assertTrue(self.client.post.call_args.kwargs["result"])
        self.assertEqual(self.client.post.call_args.args[0]["workflow_id"], saved["workflow_id"])

    def test_completed_trial_is_reused_without_api_calls(self):
        with RemoteStudy(self.output, spec(), self.client) as remote:
            remote.evaluate("trial", {})
        self.client.reset_mock()
        with RemoteStudy(self.output, spec(), self.client, resume=True) as remote:
            self.assertEqual(remote.evaluate("trial", {}), 0.75)
        self.client.post.assert_not_called()

    def test_resume_rejects_changed_inputs_and_trial_config(self):
        with RemoteStudy(self.output, spec(), self.client) as remote:
            remote.evaluate("trial", {})
        with self.assertRaisesRegex(ValueError, "Cannot resume"):
            RemoteStudy(self.output, spec(dataset="Other"), self.client, resume=True)
        with RemoteStudy(self.output, spec(), self.client, resume=True) as remote:
            with self.assertRaisesRegex(ValueError, "different config"):
                remote.evaluate("trial", {"new": True})

    def test_concurrent_driver_on_same_checkpoint_is_rejected(self):
        with RemoteStudy(self.output, spec(), self.client):
            with self.assertRaises(sqlite3.OperationalError):
                RemoteStudy(self.output, spec(), self.client, resume=True)

    def test_failed_workflow_stops_and_cannot_be_silently_restarted(self):
        self.client.post.side_effect = lambda attrs, result=False, **_: (
            completed(status="failed", error="testbench failed") if result else {"id": attrs["workflow_id"]}
        )
        with (
            self.assertRaisesRegex(RuntimeError, "testbench failed"),
            RemoteStudy(self.output, spec(), self.client) as remote,
        ):
            remote.evaluate("trial", {})
        self.assertEqual(read_json(self.output / "report.json")["status"], "incomplete")
        with RemoteStudy(self.output, spec(), self.client, resume=True) as remote:
            with self.assertRaisesRegex(RuntimeError, "previously failed"):
                remote.evaluate("trial", {})

    def test_invalid_or_incomplete_results_are_not_scored(self):
        bad_results = [
            completed(dataset_version=0),
            completed(dataset_version=8),
            completed(metrics_json='{"f1": "NaN"}'),
            completed(metrics_json='{"f1": 0.8, "timed_out": 0.1}'),
        ]
        for index, result in enumerate(bad_results):
            with self.subTest(result=result):
                self.client.post.side_effect = lambda attrs, result=False, response=result, **_: (
                    response if result else {"id": attrs["workflow_id"]}
                )
                with (
                    self.assertRaises(RuntimeError),
                    RemoteStudy(self.output / str(index), spec(dataset_version=7), self.client) as remote,
                ):
                    remote.evaluate("trial", {})

    def test_poll_timeout_preserves_submitted_workflow(self):
        self.client.post.side_effect = lambda attrs, result=False, **_: (
            {"completed": False, "status": "running"} if result else {"id": attrs["workflow_id"]}
        )
        with (
            patch("tasks.libs.anomalydetection.ablation.time.monotonic", side_effect=[0, 0, 1, 2]),
            patch("tasks.libs.anomalydetection.ablation.time.sleep"),
            self.assertRaises(TimeoutError),
            RemoteStudy(self.output, spec(), self.client, timeout=1) as remote,
        ):
            remote.evaluate("trial", {})
        self.assertEqual(read_json(self.output / "study.json")["trials"]["trial"]["status"], "submitted")

    def test_only_result_poll_is_retried(self):
        initial = self.client.post.side_effect
        calls = 0

        def post(attrs, *, result=False, **kwargs):
            nonlocal calls
            calls += 1
            if calls == 2:
                raise TransientError("HTTP 503")
            return initial(attrs, result=result, **kwargs)

        self.client.post.side_effect = post
        with (
            patch("tasks.libs.anomalydetection.ablation.time.sleep"),
            RemoteStudy(self.output, spec(), self.client) as remote,
        ):
            remote.evaluate("trial", {})
        self.assertEqual(len(self.requests), 1)
        self.assertEqual(calls, 3)

    def test_f1_zero_is_valid_but_nonfinite_and_boolean_values_are_not(self):
        self.assertEqual(metric({"f1": 0}, "f1"), 0)
        for value in (True, float("inf"), float("nan"), -1, 2):
            with self.subTest(value=value), self.assertRaises(ValueError):
                metric({"f1": value}, "f1")


class TestAblationArtifacts(unittest.TestCase):
    def test_publish_manifest_matches_uploaded_binary(self):
        with tempfile.TemporaryDirectory() as directory, chdir(directory):
            binary = Path("bin/anomalydetection-testbench")
            binary.parent.mkdir()
            elf = bytearray(20)
            elf[:6] = b"\x7fELF\x02\x01"
            elf[18:20] = (62).to_bytes(2, "little")
            binary.write_bytes(elf)
            ctx = Mock()
            with (
                patch.dict("os.environ", {"CI_COMMIT_SHA": "a" * 40}),
                patch("tasks.anomalydetection._build_testbench") as build,
            ):
                publish_ddeval_testbench.body(ctx)
            build.assert_called_once_with(ctx, env={"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"})
            artifact = read_json(Path("observer-ddeval-testbench.json"))["executor_config"]["binary_artifacts"][
                "testbench"
            ]
            self.assertIn(f"/official-releases/{'a' * 40}/{artifact['sha256']}/", artifact["uri"])
            self.assertIn(artifact["uri"], ctx.run.call_args.args[0])
            self.assertIn(artifact["sha256"], Path("observer-ddeval-testbench.env").read_text())

    def test_restore_only_reads_checkpoint_artifacts_and_excludes_sqlite_lock(self):
        archive = io.BytesIO()
        with zipfile.ZipFile(archive, "w") as zipped:
            zipped.writestr("observer-ablation-ddeval/study.json", '{"id": "study"}')
            zipped.writestr("observer-ablation-ddeval/.lock.sqlite", b"ignored")
            zipped.writestr("unrelated.txt", "ignored")
        response = Mock()
        response.iter_content.return_value = [archive.getvalue()]
        get = MagicMock()
        get.return_value.__enter__.return_value = response
        with (
            tempfile.TemporaryDirectory() as directory,
            patch.dict(
                "os.environ",
                {"CI_API_V4_URL": "https://gitlab.example/api/v4", "CI_PROJECT_ID": "1", "CI_JOB_TOKEN": "fixture"},
            ),
            patch("tasks.libs.anomalydetection.ablation_ci.requests.get", get),
        ):
            output = Path(directory) / "observer-ablation-ddeval"
            restore_checkpoint("123", output)
            self.assertEqual(read_json(output / "study.json"), {"id": "study"})
            self.assertFalse((output / ".lock.sqlite").exists())
            self.assertFalse((Path(directory) / "unrelated.txt").exists())


@unittest.skipUnless(
    importlib.util.find_spec("optuna"), "Run with dda inv --dep optuna for the optimizer integration test"
)
class TestAblationHTTPIntegration(unittest.TestCase):
    def test_search_tune_and_resume_use_real_http_without_duplicate_submissions(self):
        requests_seen = []
        experiments = {}

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):  # noqa: called by BaseHTTPRequestHandler
                pass

            def do_POST(self):  # noqa: dispatched by BaseHTTPRequestHandler
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                attrs = body["data"]["attributes"]
                requests_seen.append((self.path, deepcopy(attrs)))
                workflow = attrs["workflow_id"]
                if self.path.endswith("/result"):
                    config = json.loads(experiments[workflow]["experiment_config"])
                    label = config["input_parameters"]["trial_metadata"]["trial"]
                    # Tuning can underperform the search: retain the best observed result.
                    score = 0.5 if label.startswith("tune/") else 0.75
                    data = completed(metrics_json=json.dumps({"f1": score, "precision": 0.8, "recall": 0.7}))
                else:
                    if workflow in experiments:
                        self.send_error(409)
                        return
                    experiments[workflow] = attrs
                    data = {"run_id": "run-1"}
                encoded = json.dumps({"data": {"id": workflow, "attributes": data}}).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(encoded)))
                self.end_headers()
                self.wfile.write(encoded)

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            client = AgentCIClient(lambda: "Bearer fixture", url=f"http://127.0.0.1:{server.server_port}/eval")
            with tempfile.TemporaryDirectory() as directory:
                output = Path(directory)
                # Go beyond TPE's initial random trials to exercise deterministic replay.
                settings = {
                    "eval_backend": "ddeval",
                    "output_dir": str(output),
                    "n_combos": 1,
                    "n_trials_search": 12,
                    "n_trials_tune": 1,
                    "ddeval_limit": 1,
                    "seed": 42,
                    "ddeval_testbench_binary_s3_uri": "s3://bucket/testbench",
                    "ddeval_testbench_sha256": "a" * 64,
                }
                ctx = Mock()
                ctx.run.return_value.stdout = "test-commit\n"
                with patch("tasks.anomalydetection.AgentCIClient", return_value=client):
                    report = eval_pipeline.body(ctx, **settings)
                self.assertEqual(len(experiments), 13)
                self.assertEqual(len(report["searches"]), 1)
                self.assertEqual(report["dataset_version"], 7)
                self.assertEqual(report["best"]["score"], 0.75)
                self.assertTrue(report["best"]["trial"].startswith("search/"))
                before = len(requests_seen)
                with patch("tasks.anomalydetection.AgentCIClient", return_value=client):
                    resumed = eval_pipeline.body(ctx, **settings, resume=True)
                self.assertEqual(len(requests_seen), before)
                self.assertEqual(report["best"], resumed["best"])
                self.assertTrue((output / "best_config.json").exists())
                self.assertIn("Precision", (output / "summary.md").read_text())
                versions = [a["dataset_version"] for path, a in requests_seen if not path.endswith("/result")]
                self.assertEqual(versions, [0] + [7] * 12)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()
