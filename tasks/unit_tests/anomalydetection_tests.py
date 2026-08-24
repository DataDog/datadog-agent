import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from tasks.anomalydetection import _AgentCIAPITransientError, _run_agent_ci_api_ddeval_workflow


class TestAgentCIAPIDDEvalWorkflow(unittest.TestCase):
    def test_retries_transient_result_poll_without_starting_another_workflow(self):
        options = SimpleNamespace(
            max_attempts=1,
            jobs=6,
            limit=0,
            where_in="",
            agent_ci_api_timeout=60,
            agent_ci_api_poll_wait=0,
            agent_ci_api_poll_interval=1,
        )
        responses = [
            {"id": "workflow-1", "run_id": "run-1"},
            _AgentCIAPITransientError("agent-ci-api request failed with code 503"),
            {"completed": True, "status": "completed", "metrics_json": "{}"},
        ]

        with tempfile.NamedTemporaryFile() as log_file:
            with (
                patch("tasks.anomalydetection._agent_ci_api_post", side_effect=responses) as post,
                patch("tasks.anomalydetection.time.monotonic", return_value=0),
                patch("tasks.anomalydetection.time.sleep") as sleep,
            ):
                result, workflow_id, run_id = _run_agent_ci_api_ddeval_workflow(
                    None,
                    experiment_config={"components": {}},
                    options=options,
                    logger=MagicMock(),
                    log_path=log_file.name,
                )

        self.assertEqual((workflow_id, run_id), ("workflow-1", "run-1"))
        self.assertEqual(result["status"], "completed")
        self.assertEqual(post.call_count, 3)
        self.assertEqual(post.call_args_list[0].args[2], "observer-ablation/eval")
        self.assertEqual(post.call_args_list[1].args[2], "observer-ablation/eval/result")
        self.assertEqual(post.call_args_list[2].args[2], "observer-ablation/eval/result")
        sleep.assert_called_once_with(1)
