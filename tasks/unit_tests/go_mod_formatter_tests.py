import os
import shutil
import subprocess
import unittest


def run_mod_formatter(path, formatFile=False, allow_fail=False, repo_path=None):
    repo_path = repo_path or os.getcwd()
    # Use os.path.isabs for cross-platform absolute path check (Windows: C:\, Unix: /)
    if not os.path.isabs(path):
        path = os.path.abspath(path)

    # Build the command with properly quoted paths for cross-platform compatibility
    modformatter_path = os.path.join(".", "internal", "tools", "modformatter", "modformatter.go")
    cmd = ["go", "run", modformatter_path, "--path", path, "--repoPath", repo_path]
    if formatFile:
        cmd.extend(["--formatFile", "true"])

    proc = subprocess.run(
        cmd,
        capture_output=True,
    )
    if not allow_fail:
        assert proc.returncode == 0, f"\n{proc.stderr.decode('utf-8')}"
    return proc.stdout


def setup_format_test(src, dest):
    # Use Python's os.remove for cross-platform compatibility (instead of Unix 'rm')
    dest_file = './tasks/unit_tests/testdata/go_mod_formatter/format_package_test/go.mod'
    if os.path.exists(dest_file):
        os.remove(dest_file)
    # Use Python's shutil.copy for cross-platform compatibility (instead of Unix 'cp')
    shutil.copy(src, dest)


class TestGoModFormatter(unittest.TestCase):
    def test_valid_go_mod_check(self):
        output = run_mod_formatter("./tasks/unit_tests/testdata/go_mod_formatter/valid_package/")
        assert len(output) == 0, output

    def test_invalid_go_mod_check(self):
        output = run_mod_formatter("./tasks/unit_tests/testdata/go_mod_formatter/invalid_package/", allow_fail=True)
        assert len(output) != 0, output

    def test_valid_go_mod_format(self):
        setup_format_test(
            "./tasks/unit_tests/testdata/go_mod_formatter/valid_package/go.mod",
            "./tasks/unit_tests/testdata/go_mod_formatter/format_package_test/go.mod",
        )
        output = run_mod_formatter("./tasks/unit_tests/testdata/go_mod_formatter/format_package_test/", formatFile=True)
        assert len(output) == 0, output
        output = run_mod_formatter("./tasks/unit_tests/testdata/go_mod_formatter/format_package_test/")
        assert len(output) == 0, output
        # Use Python's os.remove for cross-platform compatibility (instead of Unix 'rm')
        os.remove('./tasks/unit_tests/testdata/go_mod_formatter/format_package_test/go.mod')

    def test_invalid_go_mod_format(self):
        setup_format_test(
            "./tasks/unit_tests/testdata/go_mod_formatter/invalid_package/go.mod",
            "./tasks/unit_tests/testdata/go_mod_formatter/format_package_test/go.mod",
        )
        output = run_mod_formatter("./tasks/unit_tests/testdata/go_mod_formatter/format_package_test/", formatFile=True)
        assert len(output) != 0, output
        output = run_mod_formatter(
            "./tasks/unit_tests/testdata/go_mod_formatter/format_package_test/", formatFile=True, allow_fail=True
        )
        assert len(output) == 0, output
        # Use Python's os.remove for cross-platform compatibility (instead of Unix 'rm')
        os.remove('./tasks/unit_tests/testdata/go_mod_formatter/format_package_test/go.mod')


if __name__ == '__main__':
    unittest.main()
