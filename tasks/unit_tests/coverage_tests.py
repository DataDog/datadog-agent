import tempfile
import unittest
from contextlib import nullcontext
from pathlib import Path
from unittest.mock import MagicMock, patch

from invoke import Context

from tasks.coverage import process_e2e_coverage_folders, upload_coverage_cache


class TestUploadCoverageCache(unittest.TestCase):
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

        upload_coverage_cache.body(ctx, pull_coverage_cache=True)

        get_main_parent_commit.assert_called_once_with(ctx)
        apply_missing_coverage.assert_called_once_with(ctx, from_commit_sha='a' * 40, keep_temp_files=False)

    @patch('tasks.coverage.gitlab_section', return_value=nullcontext())
    @patch('tasks.coverage.upload_coverage_to_s3')
    @patch('tasks.coverage.os.path.exists', return_value=True)
    def test_push_coverage_cache(self, _exists, upload_coverage_to_s3, _gitlab_section):
        ctx = MagicMock(spec=Context)

        upload_coverage_cache.body(ctx, push_coverage_cache=True)

        upload_coverage_to_s3.assert_called_once_with(ctx)


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


if __name__ == '__main__':
    unittest.main()
