import unittest
import subprocess


class TestGoModFormatter(unittest.TestCase):
    def run_mod_formatter(self, path, formatFile=False):
        extraArgs = ""
        if formatFile:
            extraArgs = "--formatFile true"
        return subprocess.check_output(f"go run ./internal/tools/modformatter/modformatter.go --path {path} {extraArgs}".split(" "))

    def test_valid_go_mod_check(self):
        output = self.run_mod_formatter("./tasks/unit-tests/testdata/go_mod_formatter/valid_package/")
        print(output)
    def test_invalid_go_mod_check(self):
        pass
    def test_valid_go_mod_format(self):
        pass
    def test_invalid_go_mod_format(self):
        pass


if __name__ == '__main__':
    unittest.main()
