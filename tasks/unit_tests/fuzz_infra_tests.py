import unittest

from tasks.fuzz_infra import get_fuzz_build_command, get_fuzz_build_tags


class TestFuzzBuild(unittest.TestCase):
    def test_build_tags_include_repository_unit_test_tags(self):
        tags = get_fuzz_build_tags()

        self.assertIn("trivy", tags)
        self.assertIn("test", tags)
        self.assertIn("amd64", tags)

    def test_build_command_uses_build_tags(self):
        command = get_fuzz_build_command("FuzzConvertBOM")

        self.assertIn("-fuzz=FuzzConvertBOM$", command)
        self.assertIn("-tags=", command)
        self.assertIn("trivy", command)


if __name__ == "__main__":
    unittest.main()
