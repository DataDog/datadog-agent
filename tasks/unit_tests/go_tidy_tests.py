import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

from invoke.context import Context

from tasks import go


class TestTidy(unittest.TestCase):
    def setUp(self):
        self._cwd = os.getcwd()
        self._tmp = tempfile.TemporaryDirectory()
        os.chdir(self._tmp.name)

    def tearDown(self):
        os.chdir(self._cwd)
        self._tmp.cleanup()

    def _run_tidy(self, bazel_on_path):
        with (
            patch.object(go, "_check_valid_mods"),
            patch.object(go.shutil, "which", return_value="/usr/bin/bazel" if bazel_on_path else None),
            patch.object(go, "_bazel_tidy") as bazel_tidy,
            patch.object(go, "_go_only_tidy") as go_only_tidy,
        ):
            go.tidy(MagicMock(spec=Context))
        return bazel_tidy, go_only_tidy

    def test_bazel_workspace_uses_bazel(self):
        Path("MODULE.bazel").touch()
        bazel_tidy, go_only_tidy = self._run_tidy(bazel_on_path=True)
        bazel_tidy.assert_called_once()
        go_only_tidy.assert_not_called()

    def test_bazel_workspace_without_bazel_uses_go(self):
        Path("MODULE.bazel").touch()
        bazel_tidy, go_only_tidy = self._run_tidy(bazel_on_path=False)
        bazel_tidy.assert_not_called()
        go_only_tidy.assert_called_once()

    def test_pre_bazel_branch_uses_go_even_with_bazel(self):
        bazel_tidy, go_only_tidy = self._run_tidy(bazel_on_path=True)
        bazel_tidy.assert_not_called()
        go_only_tidy.assert_called_once()

    def test_go_only_tidy_skips_work_sync_without_go_work(self):
        ctx = MagicMock(spec=Context)
        with patch.object(go, "get_default_modules", return_value={}):
            go._go_only_tidy(ctx, verbose=False)
        ctx.run.assert_not_called()

    def test_go_only_tidy_syncs_go_work(self):
        Path("go.work").touch()
        ctx = MagicMock(spec=Context)
        with patch.object(go, "get_default_modules", return_value={}):
            go._go_only_tidy(ctx, verbose=False)
        ctx.run.assert_called_once_with("go work sync")
