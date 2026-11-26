import subprocess
import unittest
from unittest import mock

from tasks.e2e_framework import destroy, tool


def _bazel_mock(test: unittest.TestCase, stdout: str = ""):
    """Make `bazel` record its call and return a canned result."""
    patcher = mock.patch.object(tool, "bazel", return_value=subprocess.CompletedProcess([], 0, stdout, ""))
    test.addCleanup(patcher.stop)
    return patcher.start()


def _pulumi_args(bazel) -> list[str]:
    run, label, separator, *args = bazel.call_args.args
    assert (run, label, separator) == ("run", "@pulumi", "--")
    return args


class TestRunPulumi(unittest.TestCase):
    def setUp(self):
        # Neither the caller's shell nor a real config file should leak into the assertions.
        patcher = mock.patch.dict("os.environ", {}, clear=True)
        patcher.start()
        self.addCleanup(patcher.stop)
        passphrase = mock.patch.object(tool, "_local_pulumi_passphrase", return_value=None)
        passphrase.start()
        self.addCleanup(passphrase.stop)
        self.bazel = _bazel_mock(self)

    def test_project_dir_default_uses_the_e2e_framework_project(self):
        with mock.patch.object(tool, "get_pulumi_dir_flag", return_value=["-C", "/repo/test/e2e-framework/run"]):
            tool.run_pulumi("up --yes", stack="user-aws-vm")
        self.assertEqual(
            _pulumi_args(self.bazel), ["-C", "/repo/test/e2e-framework/run", "up", "--yes", "-s", "user-aws-vm"]
        )

    def test_project_dir_false_omits_the_flag(self):
        tool.run_pulumi("login --local", project_dir=False)
        self.assertEqual(_pulumi_args(self.bazel), ["login", "--local"])

    def test_project_dir_path_is_one_argument(self):
        tool.run_pulumi("version", project_dir="/tmp/a dir")
        self.assertEqual(_pulumi_args(self.bazel), ["-C", "/tmp/a dir", "version"])

    def test_empty_flag_groups_do_not_leave_blanks(self):
        # deploy interpolates a log-flags group that is empty unless logging is on.
        tool.run_pulumi(" stack init --no-select s ", project_dir=False)
        self.assertEqual(_pulumi_args(self.bazel), ["stack", "init", "--no-select", "s"])


class TestPulumiEnv(unittest.TestCase):
    def setUp(self):
        patcher = mock.patch.dict("os.environ", {}, clear=True)
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_passphrase_comes_from_the_local_config(self):
        with mock.patch.object(tool, "_local_pulumi_passphrase", return_value="from-config"):
            env = tool.pulumi_env()
        self.assertEqual(env, {"PULUMI_CONFIG_PASSPHRASE": "from-config"})

    def test_ambient_passphrase_wins(self):
        # Empty is a valid passphrase, so presence -- not truthiness -- is what counts.
        for ambient in ({"PULUMI_CONFIG_PASSPHRASE": ""}, {"PULUMI_CONFIG_PASSPHRASE_FILE": "/tmp/pass"}):
            with self.subTest(ambient=ambient), mock.patch.dict("os.environ", ambient, clear=True):
                with mock.patch.object(tool, "_local_pulumi_passphrase", return_value="from-config") as read:
                    env = tool.pulumi_env()
                self.assertNotIn("PULUMI_CONFIG_PASSPHRASE", env)
                read.assert_not_called()

    def test_overrides_win(self):
        with mock.patch.object(tool, "_local_pulumi_passphrase", return_value="from-config"):
            env = tool.pulumi_env(
                overrides={"PULUMI_CONFIG_PASSPHRASE": "explicit", "PULUMI_K8S_DELETE_UNREACHABLE": "true"}
            )
        self.assertEqual(env["PULUMI_CONFIG_PASSPHRASE"], "explicit")
        self.assertEqual(env["PULUMI_K8S_DELETE_UNREACHABLE"], "true")

    def test_caller_env_reaches_bazel_without_a_shell_prefix(self):
        bazel = _bazel_mock(self)
        with mock.patch.object(tool, "_local_pulumi_passphrase", return_value=None):
            tool.run_pulumi("up --yes", project_dir=False, env={"PULUMI_K8S_DELETE_UNREACHABLE": "true"})
        self.assertEqual(_pulumi_args(bazel), ["up", "--yes"])
        self.assertEqual(bazel.call_args.kwargs["env"]["PULUMI_K8S_DELETE_UNREACHABLE"], "true")

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
        bazel = _bazel_mock(self, stdout='[{"name": "a"}, {"name": "b"}, {"current": true}]')
        self.assertEqual(tool.pulumi_stack_names(project_dir=False), ["a", "b"])
        self.assertEqual(_pulumi_args(bazel), ["stack", "ls", "--all", "--json"])
        self.assertTrue(bazel.call_args.kwargs["capture_output"])

    def test_project_is_forwarded(self):
        bazel = _bazel_mock(self, stdout="[]")
        tool.pulumi_stack_names(project="e2elocal", project_dir=False)
        self.assertEqual(_pulumi_args(bazel), ["stack", "ls", "--all", "--json", "--project", "e2elocal"])


class TestExistingStacks(unittest.TestCase):
    def test_only_the_current_user_stacks_are_returned(self):
        with mock.patch.object(destroy, "get_stack_name_prefix", return_value="jdoe-"):
            shorts, fulls = destroy._get_existing_stacks(["jdoe-aws-vm", "jdoe-eks", "other-aws-vm"])
        self.assertEqual(shorts, ["aws-vm", "eks"])
        self.assertEqual(fulls, ["jdoe-aws-vm", "jdoe-eks"])


class TestResourceOwnerID(unittest.TestCase):
    def test_falls_back_to_the_os_user(self):
        with mock.patch.dict("os.environ", {}, clear=True), mock.patch("getpass.getuser", return_value="John.Doe"):
            self.assertEqual(tool.get_resource_owner_id(), "john-doe")

    def test_real_user_wins_over_the_os_user(self):
        env = {"REAL_USER": "john.doe"}
        with mock.patch.dict("os.environ", env, clear=True), mock.patch("getpass.getuser", return_value="bits"):
            self.assertEqual(tool.get_resource_owner_id(), "john-doe")

    def test_workspace_name_disambiguates_two_workspaces(self):
        env = {"REAL_USER": "john.doe", "WORKSPACE_NAME": "test-e2e"}
        with mock.patch.dict("os.environ", env, clear=True), mock.patch("getpass.getuser", return_value="bits"):
            self.assertEqual(tool.get_resource_owner_id(), "john-doe-test-e2e")
            self.assertEqual(tool.get_stack_name(None, "aws/vm"), "john-doe-test-e2e-aws-vm")

    def test_an_explicit_stack_name_is_normalized(self):
        # deploy used to lowercase the full name while destroy did not, so a --stack-name
        # with uppercase deployed under a name destroy could never find again.
        env = {"REAL_USER": "john.doe"}
        with mock.patch.dict("os.environ", env, clear=True):
            self.assertEqual(tool.get_stack_name("My Stack", "aws/vm"), "john-doe-my-stack")


if __name__ == "__main__":
    unittest.main()
