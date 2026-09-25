import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


@unittest.skipUnless(
    os.environ.get("BUILDIFIER_TEST_EXECUTABLE"), "Set BUILDIFIER_TEST_EXECUTABLE to test the checkout's launcher."
)
class TestBuildifierIntegration(unittest.TestCase):
    def test_public_stdin_check_and_fix_contract(self):
        executable = os.environ["BUILDIFIER_TEST_EXECUTABLE"]

        def invoke(mode, source):
            return subprocess.run(
                [executable, "-config=off", "-lint=off", "-type=bzl", f"-mode={mode}", "-"],
                input=source,
                capture_output=True,
                timeout=60,
                check=False,
            )

        formatted_source = b"value = [1, 2]\n"
        unformatted_source = b"value=[1,2]\n"
        formatted = invoke("check", formatted_source)
        self.assertEqual(formatted.returncode, 0, formatted.stderr + formatted.stdout)
        unformatted = invoke("check", unformatted_source)
        self.assertNotEqual(unformatted.returncode, 0, unformatted.stderr + unformatted.stdout)
        fixed = invoke("fix", unformatted_source)
        self.assertEqual(fixed.returncode, 0, fixed.stderr + fixed.stdout)
        self.assertTrue(fixed.stdout)
        self.assertNotEqual(fixed.stdout, unformatted_source)
        repaired = invoke("check", fixed.stdout)
        self.assertEqual(repaired.returncode, 0, repaired.stderr + repaired.stdout)

    def test_literal_paths_and_platform_diff(self):
        source = Path(os.environ["BUILDIFIER_TEST_EXECUTABLE"]).resolve()
        on_windows = os.name == "nt"
        with tempfile.TemporaryDirectory(prefix="buildifier %BUILDIFIER_TEST_LITERAL% & ") as temporary:
            root = Path(temporary)
            executable = root / source.name
            shutil.copy2(source, executable)
            if on_windows:
                shutil.copy2(source.with_suffix(""), executable.with_suffix(""))
            else:
                executable.chmod(0o755)
            fixture = root / "file %BUILDIFIER_TEST_LITERAL% & space.bzl"
            fixture.write_text("value=[1,2]\n", encoding="utf-8")
            argument = (".\\" if on_windows else "./") + fixture.name
            for mode, success in (("diff", False), ("fix", True), ("diff", True)):
                with self.subTest(mode=mode, success=success):
                    command = [str(executable), "-config=off", "-lint=off", f"-mode={mode}"]
                    if mode == "diff":
                        command.append("-diff_command=FC" if on_windows else "-diff_command=diff --unified")
                    result = subprocess.run(
                        [*command, argument],
                        cwd=root,
                        env={**os.environ, "BUILDIFIER_TEST_LITERAL": "must-not-expand"},
                        stdin=subprocess.DEVNULL,
                        capture_output=True,
                        encoding="utf-8",
                        errors="replace",
                        timeout=60,
                        check=False,
                    )
                    self.assertEqual(result.returncode == 0, success, result.stderr + result.stdout)
                    if not success:
                        self.assertIn("value=[1,2]", result.stdout)


if __name__ == "__main__":
    unittest.main()
