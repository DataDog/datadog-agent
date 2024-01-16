import unittest

from tasks.utils import clean_nested_paths


class TestUtils(unittest.TestCase):
    def test_clean_nested_paths_1(self):
        paths = [
            "./pkg/utils/toto",
            "./pkg/utils/",
            "./pkg",
            "./toto/pkg",
            "./pkg/utils/tata",
            "./comp",
            "./component",
            "./comp/toto",
        ]
        expected_paths = ["./comp", "./component", "./pkg", "./toto/pkg"]
        self.assertEqual(clean_nested_paths(paths), expected_paths)

    def test_clean_nested_paths_2(self):
        paths = [
            ".",
            "./pkg/utils/toto",
            "./pkg/utils/",
            "./pkg",
            "./toto/pkg",
            "./pkg/utils/tata",
            "./comp",
            "./component",
            "./comp/toto",
        ]
        expected_paths = ["."]
        self.assertEqual(clean_nested_paths(paths), expected_paths)


if __name__ == '__main__':
    unittest.main()
