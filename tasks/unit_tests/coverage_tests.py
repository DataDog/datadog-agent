import tempfile
import unittest
from contextlib import nullcontext
from pathlib import Path
from unittest.mock import MagicMock, patch

from invoke import Context
from invoke.exceptions import Exit

from tasks.coverage import (
    _ensure_coverage_mode,
    manage_coverage_cache,
    process_e2e_coverage_folders,
    upload_to_datadog,
)


class TestEnsureCoverageMode(unittest.TestCase):
    def test_adds_mode_to_empty_profile(self):
        with tempfile.TemporaryDirectory() as directory:
            coverage_file = Path(directory) / "coverage.out"
            coverage_file.touch()

            _ensure_coverage_mode(str(coverage_file), "-covermode=atomic")

            self.assertEqual(coverage_file.read_text(encoding='utf-8'), "mode: atomic\n")

    def test_preserves_nonempty_profile(self):
        with tempfile.TemporaryDirectory() as directory:
            coverage_file = Path(directory) / "coverage.out"
            coverage_file.write_text("mode: count\npackage/file.go:1.1,1.2 1 1\n", encoding='utf-8')

            _ensure_coverage_mode(str(coverage_file), "-covermode=atomic")

            self.assertEqual(coverage_file.read_text(encoding='utf-8'), "mode: count\npackage/file.go:1.1,1.2 1 1\n")


class TestManageCoverageCache(unittest.TestCase):
    @patch('tasks.coverage.gitlab_section', return_value=nullcontext())
    @patch('tasks.coverage.apply_missing_coverage')
    @patch('tasks.coverage.get_main_parent_commit', return_value='a' * 40)
    @patch('tasks.coverage.os.path.exists', return_value=True)
    def test_pull_coverage_cache(
        self,
        _exists,
        get_main_parent_commit,
        apply_missing_coverage,
        _gitlab_section,
    ):
        ctx = MagicMock(spec=Context)

        manage_coverage_cache.body(ctx, pull_coverage_cache=True)

        get_main_parent_commit.assert_called_once_with(ctx)
        apply_missing_coverage.assert_called_once_with(ctx, from_commit_sha='a' * 40, keep_temp_files=False)

    @patch('tasks.coverage.gitlab_section', return_value=nullcontext())
    @patch('tasks.coverage.upload_coverage_to_s3')
    @patch('tasks.coverage.os.path.exists', return_value=True)
    def test_push_coverage_cache(self, _exists, upload_coverage_to_s3, _gitlab_section):
        ctx = MagicMock(spec=Context)

        manage_coverage_cache.body(ctx, push_coverage_cache=True)

        upload_coverage_to_s3.assert_called_once_with(ctx)

    @patch('tasks.coverage.os.path.exists', return_value=True)
    def test_rejects_pull_and_push_together(self, _exists):
        ctx = MagicMock(spec=Context)

        with self.assertRaises(Exit) as raised:
            manage_coverage_cache.body(ctx, pull_coverage_cache=True, push_coverage_cache=True)

        self.assertEqual(raised.exception.code, 1)
        self.assertIn("--pull-coverage-cache", str(raised.exception))
        self.assertIn("--push-coverage-cache", str(raised.exception))


class TestUploadToDatadog(unittest.TestCase):
    @patch('tasks.coverage.subprocess.check_call')
    @patch('tasks.coverage.shutil.which', return_value=r"C:\tools\datadog-ci.cmd")
    def test_uses_platform_resolved_command(self, which, check_call):
        upload_to_datadog.body(None, coverage_file="coverage.out")

        which.assert_called_once_with("datadog-ci")
        check_call.assert_called_once_with(
            [r"C:\tools\datadog-ci.cmd", "coverage", "upload", "--format=go-coverprofile", "coverage.out"]
        )

    @patch('tasks.coverage.shutil.which', return_value=None)
    def test_fails_when_command_is_missing(self, which):
        with self.assertRaises(Exit) as raised:
            upload_to_datadog.body(None)

        which.assert_called_once_with("datadog-ci")
        self.assertEqual(raised.exception.code, 1)


class TestProcessE2ECoverageFolders(unittest.TestCase):
    @patch('tasks.coverage.gitlab_section', return_value=nullcontext())
    def test_converts_coverage_without_metadata(self, _gitlab_section):
        ctx = MagicMock(spec=Context)

        with tempfile.TemporaryDirectory() as directory:
            coverage_folder = Path(directory) / "job" / "coverage"
            coverage_folder.mkdir(parents=True)

            process_e2e_coverage_folders.body(ctx, directory)

            ctx.run.assert_called_once_with(
                f"go tool covdata textfmt -i={coverage_folder} -o={coverage_folder.parent / 'coverage.txt'}",
                echo=True,
            )
