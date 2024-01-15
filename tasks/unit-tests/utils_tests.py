import unittest

from tasks.utils import clean_nested_paths


class TestUtils(unittest.TestCase):
    def test_clean_nested_paths(self):
        paths = ["./pkg/utils/toto", "./pkg/utils/", "./pkg/", "./toto/pkg", "./pkg/utils/tata"]
        expected_paths = ["./pkg", "./toto/pkg"]
        self.assertEqual(clean_nested_paths(paths), expected_paths)


if __name__ == '__main__':
    unittest.main()
