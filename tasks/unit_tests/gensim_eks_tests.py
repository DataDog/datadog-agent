import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

from invoke.context import MockContext, Result
from invoke.exceptions import Exit

from tasks.e2e_framework.aws.gensim_eks import (
    _generate_run_id,
    fetch_gensim_eks,
    status_gensim_eks,
    submit_gensim_eks,
)


class TestGensimEksTasks(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.repo_dir = Path(self.tmpdir.name) / "gensim-episodes"
        ep_dir = self.repo_dir / "postmortems" / "authcore-pgbouncer"
        (ep_dir / "chart").mkdir(parents=True)
        (ep_dir / "episodes").mkdir(parents=True)
        (ep_dir / "play-episode.sh").write_text("#!/bin/sh\necho ok\n")
        (ep_dir / "episodes" / "pool-saturation.yaml").write_text("name: pool-saturation\n")
        (ep_dir / "episodes" / "conn-spike.yaml").write_text("name: conn-spike\n")

    def tearDown(self):
        self.tmpdir.cleanup()

    def test_generate_run_id_is_unique_across_repeated_same_day_runs(self):
        first = _generate_run_id("abcdef1234")
        second = _generate_run_id("abcdef1234")
        self.assertNotEqual(first, second)
        self.assertTrue(first.startswith("eval-"))
        self.assertIn("-abcdef1-", first)

    def test_submit_validation_rejects_invalid_settings(self):
        ctx = MockContext()
        with self.assertRaisesRegex(Exit, "latest"):
            submit_gensim_eks.body(
                ctx,
                image="docker.io/datadog/agent-dev:latest",
                episodes="authcore-pgbouncer:pool-saturation",
            )
        with self.assertRaisesRegex(Exit, "anomaly_detection_recorder"):
            submit_gensim_eks.body(
                ctx,
                image="datadog/agent:7.55.0",
                episodes="authcore-pgbouncer:pool-saturation",
            )
        with self.assertRaisesRegex(Exit, "must exceed --flush-interval"):
            submit_gensim_eks.body(
                ctx,
                image="docker.io/datadog/agent-dev:my-branch-recorder-full",
                episodes="authcore-pgbouncer:pool-saturation",
                flush_interval="5m",
                retention="5m",
            )
        with self.assertRaisesRegex(Exit, "--recording-only is only supported"):
            submit_gensim_eks.body(
                ctx,
                image="docker.io/datadog/agent-dev:my-branch-recorder-full",
                episodes="authcore-pgbouncer:pool-saturation",
                mode="live-and-record",
                recording_only=True,
            )

    @patch("tasks.e2e_framework.aws.gensim_eks._show_connection_message")
    @patch("tasks.e2e_framework.aws.gensim_eks.deploy")
    @patch("tasks.e2e_framework.aws.gensim_eks._get_app_key", return_value="app-secret")
    @patch("tasks.e2e_framework.aws.gensim_eks._get_api_key", return_value="api-secret")
    @patch("tasks.e2e_framework.aws.gensim_eks._get_local_config")
    def test_submit_propagates_recorder_settings_and_unique_run_id(
        self,
        mock_get_cfg,
        _mock_api,
        _mock_app,
        mock_deploy,
        _mock_conn,
    ):
        mock_cfg = MagicMock()
        mock_cfg.get_aws.return_value.get_account.return_value = "agent-sandbox"
        mock_get_cfg.return_value = mock_cfg
        mock_deploy.return_value = "user-custom-stack"

        ctx = MagicMock()

        def fake_run(cmd, **kwargs):
            out_stream = kwargs.get("out_stream")
            if "rev-parse --short=10 HEAD" in cmd and out_stream is not None:
                out_stream.write("1234567890\n")
            elif "status --porcelain" in cmd and out_stream is not None:
                out_stream.write("")
            return Result(stdout="", exited=0)

        ctx.run.side_effect = fake_run

        with patch.dict("os.environ", {"GENSIM_REPO_PATH": str(self.repo_dir)}):
            submit_gensim_eks.body(
                ctx,
                image="docker.io/datadog/agent-dev:my-branch-recorder-full",
                episodes="authcore-pgbouncer:pool-saturation,authcore-pgbouncer:conn-spike",
                s3_bucket="qbranch-gensim-recordings",
                namespace="bench-ns",
                stack_name="custom-stack",
                flush_interval="30s",
                retention="120m",
                recording_only=True,
                cluster_name="gensim-custom",
            )

        self.assertTrue(mock_deploy.called)
        _, kwargs = mock_deploy.call_args
        flags = kwargs["extra_flags"]
        self.assertEqual(flags["gensim:flushInterval"], "30s")
        self.assertEqual(flags["gensim:retention"], "120m")
        self.assertEqual(flags["gensim:recordingOnly"], "true")
        self.assertEqual(flags["gensim:clusterName"], "gensim-custom")
        self.assertEqual(flags["gensim:namespace"], "bench-ns")
        self.assertTrue(flags["gensim:runId"].startswith("eval-"))
        self.assertIn("-1234567-", flags["gensim:runId"])

    @patch("tasks.e2e_framework.aws.gensim_eks.tool")
    @patch("tasks.e2e_framework.aws.gensim_eks._find_kubeconfig")
    def test_status_surfaces_separated_states_and_pending_validation(self, mock_find_kc, mock_tool):
        kc = Path(self.tmpdir.name) / "kubeconfig.yaml"
        kc.write_text("apiVersion: v1\n")
        mock_find_kc.return_value = str(kc)

        status_payload = {
            "runId": "eval-20261015T120000Z-1234567-abcdef",
            "image": "docker.io/datadog/agent-dev:my-branch-recorder-full",
            "gensimSha": "1234567890",
            "overallStatus": "failed",
            "episodes": [
                {
                    "episode": "authcore-pgbouncer",
                    "scenario": "pool-saturation",
                    "status": "done",
                    "durationSeconds": 180,
                    "parquetFiles": 8,
                    "recorderPods": 2,
                    "episodeOutcome": "failure",
                    "collectionStatus": "succeeded",
                    "uploadStatus": "succeeded",
                    "validationStatus": "pending",
                    "artifactKey": "gensim-results-eval-1-authcore-pgbouncer-pool-saturation.zip",
                },
                {
                    "episode": "authcore-pgbouncer",
                    "scenario": "conn-spike",
                    "status": "failed",
                    "episodeOutcome": "success",
                    "collectionStatus": "failed",
                    "uploadStatus": "skipped",
                    "validationStatus": "pending",
                    "error": "failed to copy recorder directory from pod datadog-agent-node2",
                },
            ],
        }
        ctx = MagicMock()
        ctx.run.return_value = Result(stdout=json.dumps(status_payload), exited=0)

        status_gensim_eks.body(ctx, stack_name="custom-stack", namespace="bench-ns")

        info_lines = "\n".join(call.args[0] for call in mock_tool.info.call_args_list)
        warn_lines = "\n".join(call.args[0] for call in mock_tool.warn.call_args_list)
        self.assertIn("monitor=failure collection=succeeded upload=succeeded validation=pending pods=2", info_lines)
        self.assertIn("validation=pending. Run `dda inv aws.eks.gensim.fetch`", info_lines)
        self.assertIn("failed to copy recorder directory from pod datadog-agent-node2", warn_lines)

    @patch("tasks.e2e_framework.aws.gensim_eks._get_local_config")
    def test_fetch_protects_existing_output_and_retains_partial_on_validation_failure(self, mock_get_cfg):
        mock_get_cfg.side_effect = Exception("use fallback wrapper")
        existing = Path(self.tmpdir.name) / "existing"
        existing.mkdir()
        ctx = MagicMock()
        with self.assertRaisesRegex(Exit, "already exists"):
            fetch_gensim_eks.body(
                ctx,
                output_dir=str(existing),
                artifact_key="gensim-results-run1-ep-sc.zip",
            )

        fresh_out = Path(self.tmpdir.name) / "fresh-out"

        def fake_run(cmd, **kwargs):
            if "aws s3 cp" in cmd:
                (fresh_out / "artifact.zip").write_bytes(b"corrupt-zip")
                return Result(exited=0)
            return Result(exited=1)

        ctx.run.side_effect = fake_run
        with self.assertRaisesRegex(Exit, "Recording validation failed"):
            fetch_gensim_eks.body(
                ctx,
                output_dir=str(fresh_out),
                run_id="eval-20261015T120000Z-1234567-abcdef",
                episode="authcore-pgbouncer",
                scenario="pool-saturation",
            )
        self.assertTrue((fresh_out / "artifact.zip").exists())


if __name__ == "__main__":
    unittest.main()
