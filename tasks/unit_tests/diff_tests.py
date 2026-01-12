import unittest
from unittest.mock import MagicMock, patch

from tasks.libs.common.diff import diff


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
        mock_print.assert_any_call("/file2.txt (-1000B)")
        mock_print.assert_any_call("/file3.txt (-1000B)")

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
        mock_print.assert_any_call("/file2.txt (+1000B)")
        mock_print.assert_any_call("/file3.txt (+1000B)")

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
        mock_print.assert_any_call("/file2.txt (-500B)")
        mock_print.assert_any_call("/subdir/file3.txt (-500B)")
        mock_print.assert_any_call("/subdir/file4.txt (-500B)")

        # Files in dir2 but not in dir1
        mock_print.assert_any_call("Files in /dir2 but not in /dir1:")
        mock_print.assert_any_call("/unique.txt (+500B)")
        mock_print.assert_any_call("/other/file5.txt (+500B)")

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

    @patch('os.stat')
    @patch('os.path.islink')
    @patch('os.path.exists')
    @patch('os.walk')
    @patch('builtins.print')
    def test_sort_by_size(self, mock_print, mock_walk, mock_exists, mock_islink, mock_stat):
        """Test case where results are sorted by size when sort_by_size=True."""
        # Setup directory structure with various files
        mock_walk.side_effect = [
            # Files in dir1
            [('/dir1', [], ['file1.txt', 'file2.txt', 'file3.txt', 'removed1.txt', 'removed2.txt'])],
            # Files in dir2
            [('/dir2', [], ['file1.txt', 'file2.txt', 'file3.txt', 'new1.txt', 'new2.txt'])],
        ]

        # Control which files exist in which directories
        def exists_side_effect(path):
            return 'removed' not in path

        mock_exists.side_effect = exists_side_effect
        mock_islink.return_value = False

        # Different file sizes to test sorting
        size_map = {
            '/dir1/file1.txt': 1000,  # file1: increase by 500 bytes
            '/dir2/file1.txt': 1500,
            '/dir1/file2.txt': 3000,  # file2: increase by 2000 bytes (larger change)
            '/dir2/file2.txt': 5000,
            '/dir1/file3.txt': 1300,  # file3: decrease by 300 bytes
            '/dir2/file3.txt': 1000,
            '/dir1/removed1.txt': 800,  # Smaller removed file
            '/dir1/removed2.txt': 1500,  # Larger removed file
            '/dir2/new1.txt': 600,  # Smaller new file
            '/dir2/new2.txt': 1200,  # Larger new file
        }

        def stat_side_effect(path, follow_symlinks=None):
            self.assertEqual(follow_symlinks, False)
            stat_result = MagicMock()
            stat_result.st_size = size_map.get(path, 0)
            return stat_result

        mock_stat.side_effect = stat_side_effect

        # Call the function with sort_by_size=True
        diff('/dir1', '/dir2', sort_by_size=True)

        # Get all print calls
        print_calls = [call[0][0] if call[0] else '' for call in mock_print.call_args_list]

        # Find the indices of size mismatches in the output
        size_mismatch_indices = [i for i, call in enumerate(print_calls) if 'Size mismatch:' in str(call)]

        # Verify size mismatches are sorted by decreasing absolute change
        # file2 (+2000), file1 (+500), file3 (-300)
        self.assertIn('file2.txt', str(print_calls[size_mismatch_indices[0]]))
        self.assertIn('+1.95KiB', str(print_calls[size_mismatch_indices[0]]))
        self.assertIn('file1.txt', str(print_calls[size_mismatch_indices[1]]))
        self.assertIn('+500B', str(print_calls[size_mismatch_indices[1]]))
        self.assertIn('file3.txt', str(print_calls[size_mismatch_indices[2]]))
        self.assertIn('-300B', str(print_calls[size_mismatch_indices[2]]))

        # Find the section for removed files
        removed_section_start = None
        for i, call in enumerate(print_calls):
            if 'Files in /dir1 but not in /dir2:' in str(call):
                removed_section_start = i
                break

        # Verify removed files are sorted by size (removed2: 1500, removed1: 800)
        self.assertIsNotNone(removed_section_start, "Could not find removed files section")
        if removed_section_start is not None:
            removed_files = []
            for i in range(removed_section_start + 1, len(print_calls)):
                if print_calls[i] and '/removed' in str(print_calls[i]):
                    removed_files.append(str(print_calls[i]))
                elif print_calls[i] and 'Files in' in str(print_calls[i]):
                    break

        self.assertEqual(len(removed_files), 2)
        self.assertIn('removed2.txt', removed_files[0])
        self.assertIn('removed1.txt', removed_files[1])

        # Find the section for new files
        new_section_start = None
        for i, call in enumerate(print_calls):
            if 'Files in /dir2 but not in /dir1:' in str(call):
                new_section_start = i
                break

        # Verify new files are sorted by size (new2: 1200, new1: 600)
        self.assertIsNotNone(new_section_start, "Could not find new files section")
        if new_section_start is not None:
            new_files = []
            for i in range(new_section_start + 1, len(print_calls)):
                if print_calls[i] and '/new' in str(print_calls[i]):
                    new_files.append(str(print_calls[i]))

            self.assertEqual(len(new_files), 2)
            self.assertIn('new2.txt', new_files[0])
            self.assertIn('new1.txt', new_files[1])
