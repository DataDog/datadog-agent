import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from python.runfiles import runfiles

# noinspection SpellCheckingInspection
stdioredirect = runfiles.Create().Rlocation(sys.argv.pop(1))


class StdioRedirectTest(unittest.TestCase):
    def test_missing_cmd_fails(self):
        self.assertEqual(subprocess.call([stdioredirect]), 1)

    def test_exit_code_propagates(self):
        self.assertEqual(subprocess.call([stdioredirect, sys.executable, "-c", "exit(42)"]), 42)

    def test_optional_double_dash_is_skipped(self):
        self.assertEqual(
            subprocess.check_output(
                [stdioredirect, "--", sys.executable, "-c", "print('hello')"],
                text=True,
            ),
            "hello\n",
        )

    def test_stdin(self):
        with tempfile.TemporaryDirectory() as d:
            inp = Path(d) / "inp"
            inp.write_text("hello\n")
            self.assertEqual(
                subprocess.check_output(
                    [
                        stdioredirect,
                        f"--stdin={inp}",
                        sys.executable,
                        "-c",
                        "import sys; sys.stdout.write(sys.stdin.read().upper())",
                    ],
                    text=True,
                ),
                "HELLO\n",
            )

    def test_stdout(self):
        with tempfile.TemporaryDirectory() as d:
            out = Path(d) / "out"
            subprocess.check_call([stdioredirect, f"--stdout={out}", sys.executable, "-c", "print('hello')"])
            self.assertEqual(out.read_text(), "hello\n")

    def test_stderr(self):
        with tempfile.TemporaryDirectory() as d:
            err = Path(d) / "err"
            subprocess.check_call(
                [
                    stdioredirect,
                    f"--stderr={err}",
                    sys.executable,
                    "-c",
                    "import sys; print('oops', file=sys.stderr)",
                ]
            )
            self.assertEqual(err.read_text(), "oops\n")

    def test_stdin_stdout(self):
        with tempfile.TemporaryDirectory() as d:
            inp, out = Path(d) / "inp", Path(d) / "out"
            inp.write_text("hello\n")
            subprocess.check_call(
                [
                    stdioredirect,
                    f"--stdin={inp}",
                    f"--stdout={out}",
                    sys.executable,
                    "-c",
                    "import sys; sys.stdout.write(sys.stdin.read().upper())",
                ]
            )
            self.assertEqual(out.read_text(), "HELLO\n")

    def test_stdout_stderr(self):
        with tempfile.TemporaryDirectory() as d:
            out, err = Path(d) / "out", Path(d) / "err"
            subprocess.check_call(
                [
                    stdioredirect,
                    f"--stdout={out}",
                    f"--stderr={err}",
                    sys.executable,
                    "-c",
                    "import sys; print('hello'); print('oops', file=sys.stderr)",
                ]
            )
            self.assertEqual(out.read_text(), "hello\n")
            self.assertEqual(err.read_text(), "oops\n")


if __name__ == "__main__":
    unittest.main()
