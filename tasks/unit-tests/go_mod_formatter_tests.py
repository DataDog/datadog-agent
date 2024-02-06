import unittest
import subprocess
import os


def run_mod_formatter(path, formatFile=False, allow_fail=False):
    if path[0] != "/":
        path = os.path.abspath(path)
    extraArgs = ""
    if formatFile:
        extraArgs = "--formatFile true"
    proc = subprocess.run(
        f"go run ./internal/tools/modformatter/modformatter.go --path {path} {extraArgs}",
        shell=True,
        capture_output=True,
    )
    if not allow_fail:
        assert proc.returncode == 0, f"\n{proc.stderr.decode('utf-8')}"
    return proc.stdout


def setup_format_test(src, dest):
    subprocess.run(
        'rm ./tasks/unit-tests/testdata/go_mod_formatter/format_package_test/go.mod', shell=True, capture_output=True
    )
    p = subprocess.run(f"cp {src} {dest}", shell=True)
    assert p.returncode == 0


class TestGoModFormatter(unittest.TestCase):
    def test_valid_go_mod_check(self):
        output = run_mod_formatter("./tasks/unit-tests/testdata/go_mod_formatter/valid_package/")
        assert len(output) == 0, output

    def test_invalid_go_mod_check(self):
        output = run_mod_formatter("./tasks/unit-tests/testdata/go_mod_formatter/invalid_package/", allow_fail=True)
        assert len(output) != 0, output

    def test_valid_go_mod_format(self):
        setup_format_test(
            "./tasks/unit-tests/testdata/go_mod_formatter/valid_package/go.mod",
            "./tasks/unit-tests/testdata/go_mod_formatter/format_package_test/go.mod",
        )
        output = run_mod_formatter("./tasks/unit-tests/testdata/go_mod_formatter/format_package_test/", formatFile=True)
        assert len(output) == 0, output
        output = run_mod_formatter("./tasks/unit-tests/testdata/go_mod_formatter/format_package_test/")
        assert len(output) == 0, output
        subprocess.run('rm ./tasks/unit-tests/testdata/go_mod_formatter/format_package_test/go.mod', shell=True)

    def test_invalid_go_mod_format(self):
        setup_format_test(
            "./tasks/unit-tests/testdata/go_mod_formatter/invalid_package/go.mod",
            "./tasks/unit-tests/testdata/go_mod_formatter/format_package_test/go.mod",
        )
        output = run_mod_formatter("./tasks/unit-tests/testdata/go_mod_formatter/format_package_test/", formatFile=True)
        assert len(output) != 0, output
        output = run_mod_formatter(
            "./tasks/unit-tests/testdata/go_mod_formatter/format_package_test/", formatFile=True, allow_fail=True
        )
        assert len(output) == 0, output
        subprocess.run('rm ./tasks/unit-tests/testdata/go_mod_formatter/format_package_test/go.mod', shell=True)


if __name__ == '__main__':
    unittest.main()
