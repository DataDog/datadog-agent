import unittest
from unittest.mock import MagicMock, call, mock_open, patch

import requests

from tasks.libs.common.download import download


class TestDownload(unittest.TestCase):
    @patch('os.path.isdir')
    @patch('builtins.open', new_callable=mock_open)
    @patch('requests.get')
    @patch('rich.progress.Progress')
    def test_download_to_file(self, mock_progress, mock_get, mock_open, mock_isdir):
        # Setup mocks
        mock_isdir.return_value = False

        # Create mock response
        mock_response = MagicMock()
        mock_response.headers.get.return_value = '1000'
        mock_response.iter_content.return_value = [b'chunk1', b'chunk2', b'chunk3']
        mock_get.return_value = mock_response

        # Create mock progress context
        mock_progress_context = MagicMock()
        mock_progress_instance = mock_progress_context.__enter__.return_value
        mock_progress_instance.add_task.return_value = 'task1'
        mock_progress.return_value = mock_progress_context

        # Call the function
        url = 'https://example.com/file.txt'
        path = '/path/to/file.txt'
        result = download(url, path)

        # Verify results
        self.assertEqual(result, path)

        # Verify requests.get was called with the right parameters
        mock_get.assert_called_once_with(url, stream=True, timeout=None)
        mock_response.raise_for_status.assert_called_once()

        # Verify file was opened
        mock_open.assert_called_once_with(path, 'wb')

        # Verify content was written
        mock_file = mock_open()
        expected_write_calls = [call(b'chunk1'), call(b'chunk2'), call(b'chunk3')]
        self.assertEqual(mock_file.write.call_args_list, expected_write_calls)

        # Verify progress was updated
        mock_progress_instance.add_task.assert_called_once_with('Downloading file.txt', total=1000)
        self.assertEqual(mock_progress_instance.update.call_count, 3)
        for i, chunk in enumerate([b'chunk1', b'chunk2', b'chunk3']):
            self.assertEqual(mock_progress_instance.update.call_args_list[i], call('task1', advance=len(chunk)))

    @patch('os.path.isdir')
    @patch('builtins.open', new_callable=mock_open)
    @patch('requests.get')
    @patch('rich.progress.Progress')
    def test_download_to_directory(self, mock_progress, mock_get, mock_open, mock_isdir):
        # Setup mocks
        mock_isdir.return_value = True

        # Create mock response
        mock_response = MagicMock()
        mock_response.headers.get.return_value = '500'
        mock_response.iter_content.return_value = [b'data']
        mock_get.return_value = mock_response

        # Create mock progress context
        mock_progress_context = MagicMock()
        mock_progress_instance = mock_progress_context.__enter__.return_value
        mock_progress_instance.add_task.return_value = 'task1'
        mock_progress.return_value = mock_progress_context

        # Call the function
        url = 'https://example.com/file.txt'
        path = '/path/to/dir'
        result = download(url, path)

        # Verify results
        self.assertEqual(result, '/path/to/dir/file.txt')

        # Verify file operations
        mock_open.assert_called_once_with('/path/to/dir/file.txt', 'wb')

    @patch('os.path.isdir')
    @patch('builtins.open', new_callable=mock_open)
    @patch('requests.get')
    def test_download_http_error(self, mock_get, mock_open, mock_isdir):
        # Setup mocks
        mock_isdir.return_value = False

        # Configure the response to raise an HTTP error
        mock_response = MagicMock()
        mock_response.raise_for_status.side_effect = requests.exceptions.HTTPError("404 Not Found")
        mock_get.return_value = mock_response

        # Call the function and expect an exception
        url = 'https://example.com/nonexistent.txt'
        path = '/path/to/file.txt'

        with self.assertRaises(requests.exceptions.HTTPError):
            download(url, path)

        # Verify the file was not opened
        mock_open.assert_not_called()

    @patch('os.path.isdir')
    @patch('builtins.open', new_callable=mock_open)
    @patch('requests.get')
    @patch('rich.progress.Progress')
    def test_download_no_content_length(self, mock_progress, mock_get, mock_open, mock_isdir):
        # Setup mocks
        mock_isdir.return_value = False

        # Create mock response with no content-length header
        mock_response = MagicMock()
        mock_response.headers.get.return_value = 0  # No content length
        mock_response.iter_content.return_value = [b'some', b'content']
        mock_get.return_value = mock_response

        # Create mock progress context
        mock_progress_context = MagicMock()
        mock_progress_instance = mock_progress_context.__enter__.return_value
        mock_progress_instance.add_task.return_value = 'task1'
        mock_progress.return_value = mock_progress_context

        # Call the function
        url = 'https://example.com/unknown_size.txt'
        path = '/path/to/file.txt'
        result = download(url, path)

        # Verify results
        self.assertEqual(result, path)

        # Verify progress was configured with None for the total
        mock_progress_instance.add_task.assert_called_once_with('Downloading file.txt', total=None)
