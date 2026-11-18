import unittest
from unittest.mock import Mock, patch

from tasks.setup import pre_commit


class TestSetupHooks(unittest.TestCase):
    def test_installs_environments_and_removes_the_old_push_hook(self):
        ctx = Mock()
        ctx.run.return_value = Mock(ok=True, stdout="")
        pre_commit.body(ctx, interactive=False)
        ctx.run.assert_any_call("pre-commit uninstall --hook-type pre-push", hide=True)
        ctx.run.assert_any_call("pre-commit install --install-hooks --config .pre-commit-config.yaml", hide=True)

    def test_restores_the_global_hooks_path_if_installation_fails(self):
        ctx = Mock()
        hooks_path = "C:/Hooks with spaces"
        ctx.run.side_effect = [
            Mock(ok=True),
            Mock(stdout=hooks_path),
            Mock(),
            Mock(),
            Mock(),
            RuntimeError("installation failed"),
            Mock(),
        ]
        with patch("tasks.setup.join_command", return_value="restore global hooks path") as join:
            with self.assertRaisesRegex(RuntimeError, "installation failed"):
                pre_commit.body(ctx, interactive=False)
        join.assert_called_once_with(["git", "config", "--global", "core.hooksPath", hooks_path])
        ctx.run.assert_called_with("restore global hooks path", hide=True)
