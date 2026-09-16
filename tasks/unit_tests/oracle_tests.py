import os
import unittest
from unittest.mock import MagicMock, patch

from invoke import Context
from invoke.exceptions import Exit

from tasks import oracle


@patch.dict(os.environ, {}, clear=True)
class OracleTestTaskTests(unittest.TestCase):
    def setUp(self):
        self.ctx = MagicMock(spec=Context)
        self.start = self.enterContext(patch("tasks.oracle.start_docker"))
        self.clean = self.enterContext(patch("tasks.oracle.clean"))
        self.enterContext(patch("tasks.oracle.schema_codegen"))

    def test_local_database_is_cleaned_up(self):
        oracle.test.body(self.ctx)
        self.start.assert_called_once_with(self.ctx, False)
        self.clean.assert_called_once_with(self.ctx, False)
        command = self.ctx.run.call_args.args[0]
        self.assertIn("-count=1", command)
        self.assertIn("-timeout=20m", command)

    def test_failed_start_is_cleaned_up(self):
        self.start.side_effect = Exit("not ready")
        with self.assertRaises(Exit):
            oracle.test.body(self.ctx)
        self.clean.assert_called_once_with(self.ctx, False)
        self.ctx.run.assert_not_called()

    def test_failed_tests_are_cleaned_up(self):
        self.ctx.run.side_effect = Exit("test failed")
        with self.assertRaises(Exit):
            oracle.test.body(self.ctx)
        self.clean.assert_called_once_with(self.ctx, False)

    def test_ci_uses_service_without_managing_docker(self):
        os.environ["CI"] = "true"
        oracle.test.body(self.ctx)
        self.start.assert_not_called()
        self.clean.assert_not_called()
        self.assertEqual(self.ctx.run.call_args.kwargs["env"]["ORACLE_TEST_SERVER"], "oracle")

    def test_external_database_is_preserved(self):
        os.environ.update(SKIP_DOCKER="1", ORACLE_TEST_SERVER="external", ORACLE_TEST_PORT="31521")
        oracle.test.body(self.ctx)
        self.start.assert_not_called()
        self.clean.assert_not_called()
        self.assertEqual(
            self.ctx.run.call_args.kwargs["env"],
            {"ORACLE_TEST_SERVER": "external", "ORACLE_TEST_PORT": "31521"},
        )


class OracleStartDockerTests(unittest.TestCase):
    def setUp(self):
        self.ctx = MagicMock(spec=Context)
        self.enterContext(patch("tasks.oracle.sleep"))

    def run_with_statuses(self, statuses):
        statuses = iter(statuses)

        def run(command, **_):
            if command == "docker compose ps -q oracle":
                return MagicMock(stdout="test-container-id\n")
            if command.startswith("docker inspect") and ".State.Health.Status" in command:
                return MagicMock(stdout=next(statuses))
            return MagicMock()

        self.ctx.run.side_effect = run
        oracle.start_docker.body(self.ctx)

    def test_waits_until_healthy(self):
        self.run_with_statuses(["running starting", "running healthy"])
        commands = [call.args[0] for call in self.ctx.run.call_args_list]
        self.assertTrue(any("test-container-id" in command for command in commands))
        self.assertFalse(any(command.startswith("docker logs") for command in commands))

    def test_failed_container_prints_diagnostics(self):
        with self.assertRaises(Exit):
            self.run_with_statuses(["exited unhealthy"])
        self.ctx.run.assert_any_call("docker logs test-container-id", warn=True)

    @patch("tasks.oracle.monotonic", side_effect=[0, 0, 601])
    def test_startup_timeout_prints_diagnostics(self, _):
        with self.assertRaises(Exit):
            self.run_with_statuses(["running starting"])
        self.ctx.run.assert_any_call("docker logs test-container-id", warn=True)
