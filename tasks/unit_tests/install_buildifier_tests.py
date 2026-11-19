import os
import unittest
from unittest.mock import patch

from invoke import Context

from tasks import install_tasks
from tasks.libs.common.utils import bin_name


class TestInstallBuildifier(unittest.TestCase):
    @patch('tasks.install_tasks.Path')
    @patch('tasks.install_tasks.get_gobin', return_value='bin')
    @patch('tasks.install_tasks.multiprocessing.Pool')
    @patch('tasks.libs.build.bazel.build_binary_with_bazel')
    def test_optional_buildifier_uses_existing_bazel_installer(self, build, pool, gobin, path):
        pool.return_value.__enter__.return_value.map.return_value = []
        path.return_value.glob.return_value = []
        ctx = Context()
        install_tasks.install_tools(ctx, max_retry=1)
        build.assert_not_called()
        install_tasks.install_tools(ctx, max_retry=1, buildifier=True)
        build.assert_called_once_with(
            '@multitool//tools/buildifier',
            args=['--lockfile_mode=off'],
            bin_path=os.path.join('bin', bin_name('buildifier')),
        )
