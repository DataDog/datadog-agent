"""Runs distrusted_spki_check.py under the built interpreter."""

import os
import subprocess
import unittest

from python.runfiles import runfiles


class DistrustedSpkiTest(unittest.TestCase):
    def test_built_interpreter_rejects_distrusted_chain(self):
        r = runfiles.Create()
        args = [r.Rlocation(os.environ[name]) for name in ("PYTHON", "CHECK_SCRIPT", "DISTRUSTED_ZIP")]
        result = subprocess.run([args[0], "-I", *args[1:]], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
