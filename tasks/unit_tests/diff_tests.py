import unittest
from unittest.mock import MagicMock, patch

from tasks.libs.common.diff import diff


# TODO(incident-41849): re-enable this test once macOS runners are back up
@unittest.SkipTest
class TestDiff(unittest.TestCase):
    @patch('os.stat')
    @patch('os.path.islink')
    @patch('os.path.exists')
    @patch('os.walk')
    @patch('builtins.print')
    def test_identical_directories(self, mock_print, mock_walk, mock_exists, mock_islink, mock_stat):
        """Test case where directories have identical files with identical sizes."""
        # Setup directory structure
        mock_walk.side_effect = [
            # Files in dir1
            [('/dir1', [], ['file1.txt', 'file2.txt'])],
            # Files in dir2
            [('/dir2', [], ['file1.txt', 'file2.txt'])],
        ]

        # All files exist in both directories
        mock_exists.return_value = True
        mock_islink.return_value = False

        # Same file sizes
        def stat_side_effect(path, follow_symlinks=None):
            self.assertEqual(follow_symlinks, False)
            stat_result = MagicMock()
            stat_result.st_size = 1000
            return stat_result

        mock_stat.side_effect = stat_side_effect

        # Call the function
        diff('/dir1', '/dir2')

        # Verify no size mismatch was printed (only the newline prints should be called)
        self.assertEqual(mock_print.call_count, 1)  # Just one newline print for identical directories
        mock_print.assert_called_with()

    @patch('os.stat')
    @patch('os.path.islink')
    @patch('os.path.exists')
    @patch('os.walk')
    @patch('builtins.print')
    def test_size_mismatch(self, mock_print, mock_walk, mock_exists, mock_islink, mock_stat):
        """Test case where files exist in both directories but have different sizes."""
        # Setup directory structure
        mock_walk.side_effect = [
            # Files in dir1
            [('/dir1', [], ['file1.txt', 'file2.txt'])],
            # Files in dir2
            [('/dir2', [], ['file1.txt', 'file2.txt'])],
        ]

        # All files exist in both directories
        mock_exists.return_value = True
        mock_islink.return_value = False

        # Different file sizes based on file path
        def stat_side_effect(path, follow_symlinks=None):
            self.assertEqual(follow_symlinks, False)
            stat_result = MagicMock()
            if 'file1.txt' in path:
                if '/dir1' in path:
                    stat_result.st_size = 1000
                else:
                    stat_result.st_size = 1500
            elif 'file2.txt' in path:
                if '/dir1' in path:
                    stat_result.st_size = 2000
                else:
                    stat_result.st_size = 1800
            return stat_result

        mock_stat.side_effect = stat_side_effect

        # Call the function
        diff('/dir1', '/dir2')

        # Verify size mismatches were printed
        mock_print.assert_any_call("Size mismatch: /file1.txt 1000 vs 1500 (+500B)")
        mock_print.assert_any_call("Size mismatch: /file2.txt 2000 vs 1800 (-200B)")

    @patch('os.stat')
    @patch('os.path.islink')
    @patch('os.path.exists')
    @patch('os.walk')
    @patch('builtins.print')
    def test_files_in_dir1_not_in_dir2(self, mock_print, mock_walk, mock_exists, mock_islink, mock_stat):
        """Test case where some files exist in dir1 but not in dir2."""
        # Setup directory structure
        mock_walk.side_effect = [
            # Files in dir1
            [('/dir1', [], ['file1.txt', 'file2.txt', 'file3.txt'])],
            # Files in dir2
            [('/dir2', [], ['file1.txt'])],
        ]

        # Only file1.txt exists in both
        def exists_side_effect(path):
            return 'file1.txt' in path or not path.startswith('/dir2')

        mock_exists.side_effect = exists_side_effect
        mock_islink.return_value = False

        # Same file sizes for the common file
        def stat_side_effect(path, follow_symlinks=None):
            self.assertEqual(follow_symlinks, False)
            stat_result = MagicMock()
            stat_result.st_size = 1000
            return stat_result

        mock_stat.side_effect = stat_side_effect

        # Call the function
        diff('/dir1', '/dir2')

        # Verify output includes files in dir1 but not in dir2
        mock_print.assert_any_call("Files in /dir1 but not in /dir2:")
        mock_print.assert_any_call("/file2.txt")
        mock_print.assert_any_call("/file3.txt")

    @patch('os.stat')
    @patch('os.path.islink')
    @patch('os.path.exists')
    @patch('os.walk')
    @patch('builtins.print')
    def test_files_in_dir2_not_in_dir1(self, mock_print, mock_walk, mock_exists, mock_islink, mock_stat):
        """Test case where some files exist in dir2 but not in dir1."""
        # Setup directory structure
        mock_walk.side_effect = [
            # Files in dir1
            [('/dir1', [], ['file1.txt'])],
            # Files in dir2
            [('/dir2', [], ['file1.txt', 'file2.txt', 'file3.txt'])],
        ]

        # All files in dir2 exist; only some files have matching ones in dir1
        mock_exists.return_value = True
        mock_islink.return_value = False

        # Same file sizes for common files
        def stat_side_effect(path, follow_symlinks=None):
            self.assertEqual(follow_symlinks, False)
            stat_result = MagicMock()
            stat_result.st_size = 1000
            return stat_result

        mock_stat.side_effect = stat_side_effect

        # Call the function
        diff('/dir1', '/dir2')

        # Verify output includes files in dir2 but not in dir1
        mock_print.assert_any_call("Files in /dir2 but not in /dir1:")
        mock_print.assert_any_call("/file2.txt")
        mock_print.assert_any_call("/file3.txt")

    @patch('os.stat')
    @patch('os.path.islink')
    @patch('os.path.exists')
    @patch('os.walk')
    @patch('builtins.print')
    def test_complex_comparison(self, mock_print, mock_walk, mock_exists, mock_islink, mock_stat):
        """Test a complex case with various differences."""
        # Setup directory structure
        mock_walk.side_effect = [
            # Files in dir1 with subdirectories
            [
                ('/dir1', ['subdir'], ['file1.txt', 'file2.txt']),
                ('/dir1/subdir', [], ['file3.txt', 'file4.txt']),
            ],
            # Files in dir2 with different subdirectories
            [
                ('/dir2', ['other'], ['file1.txt', 'unique.txt']),
                ('/dir2/other', [], ['file5.txt']),
            ],
        ]

        # Control which files exist in which directories
        def exists_side_effect(path):
            return path in [
                '/dir2/file1.txt',  # Common file
            ]

        mock_exists.side_effect = exists_side_effect
        mock_islink.return_value = False

        # Different file sizes for the common file
        def stat_side_effect(path, follow_symlinks=None):
            self.assertEqual(follow_symlinks, False)
            stat_result = MagicMock()
            if 'file1.txt' in path:
                if '/dir1' in path:
                    stat_result.st_size = 1000
                else:
                    stat_result.st_size = 1200
            else:
                stat_result.st_size = 500  # Default for other files
            return stat_result

        mock_stat.side_effect = stat_side_effect

        # Call the function
        diff('/dir1', '/dir2')

        # Verify all expected outputs
        # Size mismatch for file1.txt
        mock_print.assert_any_call("Size mismatch: /file1.txt 1000 vs 1200 (+200B)")

        # Files in dir1 but not in dir2
        mock_print.assert_any_call("Files in /dir1 but not in /dir2:")
        mock_print.assert_any_call("/file2.txt")
        mock_print.assert_any_call("/subdir/file3.txt")
        mock_print.assert_any_call("/subdir/file4.txt")

        # Files in dir2 but not in dir1
        mock_print.assert_any_call("Files in /dir2 but not in /dir1:")
        mock_print.assert_any_call("/unique.txt")
        mock_print.assert_any_call("/other/file5.txt")

    @patch('os.stat')
    @patch('os.path.islink')
    @patch('os.path.exists')
    @patch('os.walk')
    @patch('builtins.print')
    def test_symlinks_handled_correctly(self, mock_print, mock_walk, mock_exists, mock_islink, mock_stat):
        """Test that symlinks are handled correctly."""
        # Setup directory structure
        mock_walk.side_effect = [
            # Files in dir1
            [('/dir1', [], ['file1.txt', 'symlink.txt'])],
            # Files in dir2
            [('/dir2', [], ['file1.txt', 'symlink.txt'])],
        ]

        # Control which files exist and which are symlinks
        def exists_side_effect(path):
            return True

        def islink_side_effect(path):
            return 'symlink.txt' in path

        mock_exists.side_effect = exists_side_effect
        mock_islink.side_effect = islink_side_effect

        # Same file sizes for non-symlink files
        def stat_side_effect(path, follow_symlinks=None):
            self.assertEqual(follow_symlinks, False)
            stat_result = MagicMock()
            if 'file1.txt' in path:
                stat_result.st_size = 1000
            else:
                stat_result.st_size = 0  # Default for other files
            return stat_result

        mock_stat.side_effect = stat_side_effect

        # Call the function
        diff('/dir1', '/dir2')

        # Verify symlinks were properly skipped
        self.assertEqual(mock_print.call_count, 1)  # Only the newline should be printed
