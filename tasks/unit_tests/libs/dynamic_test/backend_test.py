"""
Unit tests for dynamic test backends
"""

import json
import unittest
from unittest.mock import MagicMock, patch

from tasks.libs.dynamic_test.backend import S3Backend
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind


class TestS3Backend(unittest.TestCase):
    def setUp(self):
        self.backend = S3Backend("s3://test-bucket/prefix")

    def test_init_constructs_correct_path(self):
        backend = S3Backend("s3://bucket/some/path")
        self.assertEqual(backend.s3_base_path, "s3://bucket/some/path/dynamic_test")

    def test_init_strips_trailing_slash(self):
        backend = S3Backend("s3://bucket/path/")
        self.assertEqual(backend.s3_base_path, "s3://bucket/path/dynamic_test")

    @patch("tasks.libs.dynamic_test.backend.upload_file_to_s3")
    @patch("tempfile.mkdtemp")
    def test_upload_index_creates_correct_s3_path(self, mock_mkdtemp, mock_upload):
        mock_mkdtemp.return_value = "/tmp/test"

        index = DynamicTestIndex({"job": {"pkg": ["test1"]}})
        result = self.backend.upload_index(index, IndexKind.PACKAGE, "commit123")

        expected_path = "s3://test-bucket/prefix/dynamic_test/package/commit123/index.json"
        self.assertEqual(result, expected_path)
        mock_upload.assert_called_once_with(file_path="/tmp/test/index.json", s3_path=expected_path)

    @patch("tasks.libs.dynamic_test.backend.download_file_from_s3")
    @patch("tempfile.mkdtemp")
    def test_fetch_index_downloads_and_parses(self, mock_mkdtemp, mock_download):
        mock_mkdtemp.return_value = "/tmp/test"

        # Create a mock JSON file that will be "downloaded"
        test_data = {"job": {"pkg": ["test1"]}}
        with patch("builtins.open", create=True) as mock_open:
            mock_open.return_value.__enter__.return_value.read.return_value = json.dumps(test_data)

            result = self.backend.fetch_index(IndexKind.PACKAGE, "commit123")

            self.assertIsInstance(result, DynamicTestIndex)
            self.assertEqual(result.to_dict(), test_data)
            mock_download.assert_called_once_with(
                s3_path="s3://test-bucket/prefix/dynamic_test/package/commit123/index.json",
                local_path="/tmp/test/index.json",
            )

    @patch("tasks.libs.dynamic_test.backend.download_folder_from_s3")
    @patch("tempfile.mkdtemp")
    @patch("os.listdir")
    @patch("os.path.isdir")
    def test_consolidate_index_merges_multiple_indexes(self, mock_isdir, mock_listdir, mock_mkdtemp, mock_download):
        mock_mkdtemp.return_value = "/tmp/consolidate"
        mock_listdir.return_value = ["job1", "job2"]
        mock_isdir.return_value = True

        # Mock the index files that would be downloaded
        index1_data = {"job": {"pkg1": ["test1"]}}
        index2_data = {"job": {"pkg2": ["test2"]}}

        def mock_open_side_effect(filename, *args, **kwargs):
            mock_file = MagicMock()
            if "job1" in filename:
                mock_file.__enter__.return_value = MagicMock()
                mock_file.__enter__.return_value.__iter__ = lambda x: iter([json.dumps(index1_data)])
            elif "job2" in filename:
                mock_file.__enter__.return_value = MagicMock()
                mock_file.__enter__.return_value.__iter__ = lambda x: iter([json.dumps(index2_data)])
            return mock_file

        with patch("builtins.open", side_effect=mock_open_side_effect):
            with patch("json.load") as mock_json_load:
                mock_json_load.side_effect = [index1_data, index2_data]

                result = self.backend.consolidate_index(IndexKind.PACKAGE, "commit123")

                expected = {"job": {"pkg1": ["test1"], "pkg2": ["test2"]}}
                self.assertEqual(result.to_dict(), expected)
                mock_download.assert_called_once_with(
                    s3_path="s3://test-bucket/prefix/dynamic_test/package/commit123", local_path="/tmp/consolidate"
                )

    @patch("tasks.libs.dynamic_test.backend.list_sorted_keys_in_s3")
    def test_list_indexed_keys_filters_correctly(self, mock_list_keys):
        mock_list_keys.return_value = [
            "commit123/jobid1/index.json",
            "commit456/jobid2/index.json",
            "commit789/jobid3/index.json",
            "commit123/index.json",
            "commit456/index.json",
        ]

        result = self.backend.list_indexed_keys(IndexKind.PACKAGE)

        expected = ["commit123", "commit456"]
        self.assertEqual(result, expected)
        mock_list_keys.assert_called_once_with("s3://test-bucket/prefix/dynamic_test/package", "index.json")

    def test_list_indexed_keys_handles_empty_results(self):
        with patch("tasks.libs.dynamic_test.backend.list_sorted_keys_in_s3") as mock_list_keys:
            mock_list_keys.return_value = []

            result = self.backend.list_indexed_keys(IndexKind.PACKAGE)

            self.assertEqual(result, [])


if __name__ == "__main__":
    unittest.main()
