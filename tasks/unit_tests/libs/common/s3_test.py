"""
Unit tests for S3 utilities
"""

import os
import tempfile
import unittest
from unittest.mock import Mock, patch

from botocore.exceptions import ClientError

from tasks.libs.common.s3 import download_folder_from_s3, list_sorted_keys_in_s3, upload_file_to_s3


class TestUploadFileToS3(unittest.TestCase):
    """Test cases for upload_file_to_s3 function"""

    def setUp(self):
        """Set up test fixtures"""
        # Create a temporary file for testing
        self.temp_file = tempfile.NamedTemporaryFile(mode='w', delete=False)
        self.temp_file.write("test content")
        self.temp_file.close()
        self.temp_file_path = self.temp_file.name

    def tearDown(self):
        """Clean up test fixtures"""
        if os.path.exists(self.temp_file_path):
            os.unlink(self.temp_file_path)

    @patch('boto3.client')
    def test_upload_file_success(self, mock_boto3_client):
        """Test successful file upload to S3"""
        # Mock the S3 client
        mock_s3_client = Mock()
        mock_boto3_client.return_value = mock_s3_client

        # Call the function
        result = upload_file_to_s3(file_path=self.temp_file_path, s3_bucket="test-bucket", s3_key="test/path/file.txt")

        # Verify the result
        self.assertTrue(result)

        # Verify boto3.client was called
        mock_boto3_client.assert_called_once_with('s3')

        # Verify upload_file was called with correct parameters
        mock_s3_client.upload_file.assert_called_once_with(
            Filename=self.temp_file_path, Bucket="test-bucket", Key="test/path/file.txt"
        )

    def test_upload_file_not_found(self):
        """Test upload when file doesn't exist"""
        with self.assertRaises(FileNotFoundError):
            upload_file_to_s3(file_path="/nonexistent/file.txt", s3_path="s3://test-bucket/test/path/file.txt")

    @patch('boto3.client')
    def test_upload_file_s3_error(self, mock_boto3_client):
        """Test upload when S3 returns an error"""
        # Mock the S3 client to raise a ClientError
        mock_s3_client = Mock()
        mock_s3_client.upload_file.side_effect = ClientError(
            error_response={'Error': {'Code': 'NoSuchBucket', 'Message': 'The specified bucket does not exist'}},
            operation_name='UploadFile',
        )
        mock_boto3_client.return_value = mock_s3_client

        # Call the function and expect it to raise ClientError
        with self.assertRaises(ClientError):
            upload_file_to_s3(file_path=self.temp_file_path, s3_path="s3://nonexistent-bucket/test/path/file.txt")

    @patch('boto3.client')
    def test_upload_file_unexpected_error(self, mock_boto3_client):
        """Test upload when an unexpected error occurs"""
        # Mock the S3 client to raise a runtime error
        mock_s3_client = Mock()
        mock_s3_client.upload_file.side_effect = RuntimeError("Unexpected error")
        mock_boto3_client.return_value = mock_s3_client

        # Call the function and expect it to raise the exception
        with self.assertRaises(RuntimeError):
            upload_file_to_s3(file_path=self.temp_file_path, s3_path="s3://test-bucket/test/path/file.txt")


class TestDownloadFolderFromS3(unittest.TestCase):
    """Test cases for download_folder_from_s3 function"""

    def setUp(self):
        """Set up test fixtures"""
        # Create a temporary directory for testing
        self.temp_dir = tempfile.mkdtemp()

    def tearDown(self):
        """Clean up test fixtures"""
        import shutil

        if os.path.exists(self.temp_dir):
            shutil.rmtree(self.temp_dir)

    @patch('boto3.client')
    def test_download_folder_success(self, mock_boto3_client):
        """Test successful folder download from S3"""
        # Mock the S3 client
        mock_s3_client = Mock()
        mock_boto3_client.return_value = mock_s3_client

        # Mock paginator
        mock_paginator = Mock()
        mock_s3_client.get_paginator.return_value = mock_paginator

        # Mock page iterator with sample S3 objects
        mock_page_iterator = [
            {
                'Contents': [
                    {'Key': 'test-folder/file1.txt'},
                    {'Key': 'test-folder/file2.txt'},
                    {'Key': 'test-folder/subfolder/file3.txt'},
                ]
            }
        ]
        mock_paginator.paginate.return_value = mock_page_iterator

        # Call the function
        result = download_folder_from_s3(s3_path="s3://test-bucket/test-folder/", local_path=self.temp_dir)

        # Verify the result
        self.assertTrue(result)

        # Verify boto3.client was called
        mock_boto3_client.assert_called_once_with('s3')

        # Verify paginator was created
        mock_s3_client.get_paginator.assert_called_once_with('list_objects_v2')

        # Verify paginate was called with correct parameters
        mock_paginator.paginate.assert_called_once_with(Bucket="test-bucket", Prefix="test-folder/")

        # Verify download_file was called for each file
        expected_calls = [
            (('test-bucket', 'test-folder/file1.txt', os.path.join(self.temp_dir, 'file1.txt')),),
            (('test-bucket', 'test-folder/file2.txt', os.path.join(self.temp_dir, 'file2.txt')),),
            (('test-bucket', 'test-folder/subfolder/file3.txt', os.path.join(self.temp_dir, 'subfolder/file3.txt')),),
        ]
        self.assertEqual(mock_s3_client.download_file.call_count, 3)
        mock_s3_client.download_file.assert_has_calls(expected_calls, any_order=True)

    @patch('boto3.client')
    def test_download_folder_empty(self, mock_boto3_client):
        """Test download when S3 folder is empty"""
        # Mock the S3 client
        mock_s3_client = Mock()
        mock_boto3_client.return_value = mock_s3_client

        # Mock paginator
        mock_paginator = Mock()
        mock_s3_client.get_paginator.return_value = mock_paginator

        # Mock empty page iterator
        mock_page_iterator = [{}]  # No 'Contents' key
        mock_paginator.paginate.return_value = mock_page_iterator

        # Call the function
        result = download_folder_from_s3(s3_path="s3://test-bucket/empty-folder/", local_path=self.temp_dir)

        # Verify the result
        self.assertTrue(result)

        # Verify download_file was not called
        mock_s3_client.download_file.assert_not_called()

    @patch('boto3.client')
    def test_download_folder_s3_error(self, mock_boto3_client):
        """Test download when S3 returns an error"""
        # Mock the S3 client to raise a ClientError
        mock_s3_client = Mock()
        mock_s3_client.get_paginator.side_effect = ClientError(
            error_response={'Error': {'Code': 'NoSuchBucket', 'Message': 'The specified bucket does not exist'}},
            operation_name='ListObjects',
        )
        mock_boto3_client.return_value = mock_s3_client

        # Call the function and expect it to raise ClientError
        with self.assertRaises(ClientError):
            download_folder_from_s3(s3_path="s3://nonexistent-bucket/test-folder/", local_path=self.temp_dir)

    @patch('boto3.client')
    def test_download_folder_skip_prefix_marker(self, mock_boto3_client):
        """Test that prefix marker is skipped during download"""
        # Mock the S3 client
        mock_s3_client = Mock()
        mock_boto3_client.return_value = mock_s3_client

        # Mock paginator
        mock_paginator = Mock()
        mock_s3_client.get_paginator.return_value = mock_paginator

        # Mock page iterator with prefix marker
        mock_page_iterator = [
            {
                'Contents': [
                    {'Key': 'test-folder/'},  # This should be skipped
                    {'Key': 'test-folder/file1.txt'},  # This should be downloaded
                ]
            }
        ]
        mock_paginator.paginate.return_value = mock_page_iterator

        # Call the function
        result = download_folder_from_s3(s3_path="s3://test-bucket/test-folder/", local_path=self.temp_dir)

        # Verify the result
        self.assertTrue(result)

        # Verify download_file was called only once (for file1.txt, not for the prefix marker)
        mock_s3_client.download_file.assert_called_once_with(
            "test-bucket", "test-folder/file1.txt", os.path.join(self.temp_dir, "file1.txt")
        )


class TestListSubfoldersInS3(unittest.TestCase):
    """Test cases for list_subfolders_in_s3 function"""

    @patch('boto3.client')
    def test_list_subfolders_success(self, mock_boto3_client):
        """Test successful listing of direct subfolders"""
        # Mock the S3 client
        mock_s3_client = Mock()
        mock_boto3_client.return_value = mock_s3_client

        # Mock paginator
        mock_paginator = Mock()
        mock_s3_client.get_paginator.return_value = mock_paginator

        # Mock page iterator with CommonPrefixes (subfolders)
        mock_page_iterator = [
            {
                'CommonPrefixes': [
                    {'Prefix': 'dynamic-test/commit123/job1/'},
                    {'Prefix': 'dynamic-test/commit123/job2/'},
                    {'Prefix': 'dynamic-test/commit123/job3/'},
                ]
            }
        ]
        mock_paginator.paginate.return_value = mock_page_iterator

        # Call the function
        result = list_sorted_keys_in_s3("s3://test-bucket/dynamic-test/commit123/")

        # Verify the result
        expected_subfolders = ['job1', 'job2', 'job3']
        self.assertEqual(result, expected_subfolders)

        # Verify boto3.client was called
        mock_boto3_client.assert_called_once_with('s3')

        # Verify paginator was created with delimiter
        mock_s3_client.get_paginator.assert_called_once_with('list_objects_v2')
        mock_paginator.paginate.assert_called_once_with(
            Bucket="test-bucket", Prefix="dynamic-test/commit123/", Delimiter='/'
        )

    @patch('boto3.client')
    def test_list_subfolders_no_slash_prefix(self, mock_boto3_client):
        """Test listing subfolders when prefix doesn't end with slash"""
        # Mock the S3 client
        mock_s3_client = Mock()
        mock_boto3_client.return_value = mock_s3_client

        # Mock paginator
        mock_paginator = Mock()
        mock_s3_client.get_paginator.return_value = mock_paginator

        # Mock page iterator
        mock_page_iterator = [{'CommonPrefixes': [{'Prefix': 'dynamic-test/commit123/job1/'}]}]
        mock_paginator.paginate.return_value = mock_page_iterator

        # Call the function without trailing slash
        result = list_sorted_keys_in_s3("test-bucket/dynamic-test/commit123")

        # Verify the result
        self.assertEqual(result, ['job1'])

        # Verify paginate was called with trailing slash added
        mock_paginator.paginate.assert_called_once_with(
            Bucket="test-bucket", Prefix="dynamic-test/commit123/", Delimiter='/'
        )

    @patch('boto3.client')
    def test_list_subfolders_empty(self, mock_boto3_client):
        """Test listing when no subfolders exist"""
        # Mock the S3 client
        mock_s3_client = Mock()
        mock_boto3_client.return_value = mock_s3_client

        # Mock paginator
        mock_paginator = Mock()
        mock_s3_client.get_paginator.return_value = mock_paginator

        # Mock empty page iterator
        mock_page_iterator = [{}]  # No 'CommonPrefixes' key
        mock_paginator.paginate.return_value = mock_page_iterator

        # Call the function
        result = list_sorted_keys_in_s3("s3://test-bucket/empty-folder/")

        # Verify the result
        self.assertEqual(result, [])

    @patch('boto3.client')
    def test_list_subfolders_s3_error(self, mock_boto3_client):
        """Test listing when S3 returns an error"""
        # Mock the S3 client to raise a ClientError
        mock_s3_client = Mock()
        mock_s3_client.get_paginator.side_effect = ClientError(
            error_response={'Error': {'Code': 'NoSuchBucket', 'Message': 'The specified bucket does not exist'}},
            operation_name='ListObjects',
        )
        mock_boto3_client.return_value = mock_s3_client

        # Call the function and expect it to raise ClientError
        with self.assertRaises(ClientError):
            list_sorted_keys_in_s3("nonexistent-bucket/test-folder/")

    @patch('boto3.client')
    def test_list_subfolders_root_bucket(self, mock_boto3_client):
        """Test listing subfolders at bucket root"""
        # Mock the S3 client
        mock_s3_client = Mock()
        mock_boto3_client.return_value = mock_s3_client

        # Mock paginator
        mock_paginator = Mock()
        mock_s3_client.get_paginator.return_value = mock_paginator

        # Mock page iterator for root level
        mock_page_iterator = [{'CommonPrefixes': [{'Prefix': 'folder1/'}, {'Prefix': 'folder2/'}]}]
        mock_paginator.paginate.return_value = mock_page_iterator

        # Call the function at bucket root
        result = list_sorted_keys_in_s3("s3://test-bucket/")

        # Verify the result
        self.assertEqual(result, ['folder1', 'folder2'])

        # Verify paginate was called with empty prefix
        mock_paginator.paginate.assert_called_once_with(Bucket="test-bucket", Prefix="", Delimiter='/')


if __name__ == '__main__':
    unittest.main()
