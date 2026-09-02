import unittest
from unittest import mock

from invoke.runners import Result

from tasks.e2e_framework import destroy, tool


def _run_mock(stdout: str = "", exited: int = 0):
    """A context whose `run` records its call and returns a canned Result."""
    ctx = mock.MagicMock()
    ctx.run.return_value = Result(stdout=stdout, exited=exited)
    return ctx


def _command(ctx) -> str:
    return ctx.run.call_args.args[0]


def _env(ctx) -> dict:
    return ctx.run.call_args.kwargs["env"]


class TestRunPulumi(unittest.TestCase):
    def setUp(self):
        # Neither the caller's shell nor a real config file should leak into the assertions.
        patcher = mock.patch.dict("os.environ", {}, clear=True)
        patcher.start()
        self.addCleanup(patcher.stop)
        passphrase = mock.patch.object(tool, "_local_pulumi_passphrase", return_value=None)
        passphrase.start()
        self.addCleanup(passphrase.stop)

    def test_project_dir_default_uses_the_e2e_framework_project(self):
        ctx = _run_mock()
        with mock.patch.object(tool, "get_pulumi_dir_flag", return_value="-C /repo/test/e2e-framework/run"):
            tool.run_pulumi(ctx, "up --yes", stack="user-aws-vm")
        self.assertEqual(_command(ctx), "pulumi -C /repo/test/e2e-framework/run up --yes -s user-aws-vm")

    def test_project_dir_false_omits_the_flag(self):
        ctx = _run_mock()
        tool.run_pulumi(ctx, "login --local", project_dir=False)
        self.assertEqual(_command(ctx), "pulumi login --local")

    def test_project_dir_path_is_quoted(self):
        ctx = _run_mock()
        tool.run_pulumi(ctx, "version", project_dir="/tmp/a dir")
        self.assertEqual(_command(ctx), "pulumi -C '/tmp/a dir' version")

    def test_empty_flag_groups_do_not_leave_blanks(self):
        # deploy interpolates a log-flags group that is empty unless logging is on.
        ctx = _run_mock()
        tool.run_pulumi(ctx, " stack init --no-select s ", project_dir=False)
        self.assertEqual(_command(ctx), "pulumi stack init --no-select s")

    def test_pty_is_disabled_on_windows(self):
        ctx = _run_mock()
        with mock.patch.object(tool, "is_windows", return_value=True):
            tool.run_pulumi(ctx, "up --yes", project_dir=False, pty=True)
        self.assertFalse(ctx.run.call_args.kwargs["pty"])

    def test_pty_is_kept_elsewhere(self):
        ctx = _run_mock()
        with mock.patch.object(tool, "is_windows", return_value=False):
            tool.run_pulumi(ctx, "up --yes", project_dir=False, pty=True)
        self.assertTrue(ctx.run.call_args.kwargs["pty"])


class TestPulumiEnv(unittest.TestCase):
    def setUp(self):
        patcher = mock.patch.dict("os.environ", {}, clear=True)
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_passphrase_comes_from_the_local_config(self):
        with mock.patch.object(tool, "_local_pulumi_passphrase", return_value="from-config"):
            env = tool.pulumi_env()
        self.assertEqual(env, {"PULUMI_SKIP_UPDATE_CHECK": "true", "PULUMI_CONFIG_PASSPHRASE": "from-config"})

    def test_ambient_passphrase_wins(self):
        # Empty is a valid passphrase, so presence -- not truthiness -- is what counts.
        for ambient in ({"PULUMI_CONFIG_PASSPHRASE": ""}, {"PULUMI_CONFIG_PASSPHRASE_FILE": "/tmp/pass"}):
            with self.subTest(ambient=ambient), mock.patch.dict("os.environ", ambient, clear=True):
                with mock.patch.object(tool, "_local_pulumi_passphrase", return_value="from-config") as read:
                    env = tool.pulumi_env()
                self.assertNotIn("PULUMI_CONFIG_PASSPHRASE", env)
                read.assert_not_called()

    def test_skip_update_check_can_be_turned_off(self):
        # pulumi_version() reads the upgrade banner off stderr, so it must not be skipped.
        with mock.patch.object(tool, "_local_pulumi_passphrase", return_value=None):
            env = tool.pulumi_env(skip_update_check=False)
        self.assertEqual(env, {})

    def test_overrides_win(self):
        with mock.patch.object(tool, "_local_pulumi_passphrase", return_value="from-config"):
            env = tool.pulumi_env(
                overrides={"PULUMI_CONFIG_PASSPHRASE": "explicit", "PULUMI_K8S_DELETE_UNREACHABLE": "true"}
            )
        self.assertEqual(env["PULUMI_CONFIG_PASSPHRASE"], "explicit")
        self.assertEqual(env["PULUMI_K8S_DELETE_UNREACHABLE"], "true")

    def test_caller_env_reaches_run_without_a_shell_prefix(self):
        ctx = _run_mock()
        with mock.patch.object(tool, "_local_pulumi_passphrase", return_value=None):
            tool.run_pulumi(ctx, "up --yes", project_dir=False, env={"PULUMI_K8S_DELETE_UNREACHABLE": "true"})
        self.assertEqual(_command(ctx), "pulumi up --yes")
        self.assertEqual(_env(ctx)["PULUMI_K8S_DELETE_UNREACHABLE"], "true")

    def test_ci_selects_the_ci_backend(self):
        with (
            mock.patch.object(tool, "_local_pulumi_passphrase", return_value=None),
            mock.patch.object(tool, "_ci_pulumi_env", return_value={"PULUMI_BACKEND_URL": tool.CI_PULUMI_BACKEND_URL}),
        ):
            self.assertEqual(tool.pulumi_env(ci=True)["PULUMI_BACKEND_URL"], tool.CI_PULUMI_BACKEND_URL)
            self.assertNotIn("PULUMI_BACKEND_URL", tool.pulumi_env())

    def test_a_broken_config_does_not_break_pulumi(self):
        with mock.patch("tasks.e2e_framework.config.get_local_config", side_effect=ValueError("bad yaml")):
            self.assertIsNone(tool._local_pulumi_passphrase(None))


class TestPulumiStackNames(unittest.TestCase):
    def setUp(self):
        patcher = mock.patch.object(tool, "_local_pulumi_passphrase", return_value=None)
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_names_are_read_from_json(self):
        ctx = _run_mock(stdout='[{"name": "a"}, {"name": "b"}, {"current": true}]')
        self.assertEqual(tool.pulumi_stack_names(ctx, project_dir=False), ["a", "b"])
        self.assertEqual(_command(ctx), "pulumi stack ls --all --json")
        self.assertEqual(ctx.run.call_args.kwargs["hide"], "stdout")

    def test_project_is_forwarded(self):
        ctx = _run_mock(stdout="[]")
        tool.pulumi_stack_names(ctx, project="e2elocal", project_dir=False)
        self.assertEqual(_command(ctx), "pulumi stack ls --all --json --project e2elocal")


class TestExistingStacks(unittest.TestCase):
    def test_only_the_current_user_stacks_are_returned(self):
        with mock.patch.object(destroy, "get_stack_name_prefix", return_value="jdoe-"):
            shorts, fulls = destroy._get_existing_stacks(["jdoe-aws-vm", "jdoe-eks", "other-aws-vm"])
        self.assertEqual(shorts, ["aws-vm", "eks"])
        self.assertEqual(fulls, ["jdoe-aws-vm", "jdoe-eks"])


if __name__ == "__main__":
    unittest.main()
