"""Runs distrusted_spki_check.py under the built interpreter."""

import os
import subprocess
import unittest

from python.runfiles import runfiles


class DistrustedSpkiTest(unittest.TestCase):
    def test_built_interpreter_rejects_distrusted_chain(self):
        r = runfiles.Create()
        args = [r.Rlocation(os.environ[name]) for name in ("PYTHON", "CHECK_SCRIPT", "DISTRUSTED_ZIP")]
        lib_dirs = sorted(
            {
                os.path.dirname(r.Rlocation(lib))
                for lib in os.environ.get("SHARED_LIBS", "").split()
                if lib.endswith((".so", ".dylib")) or ".so." in lib
            }
        )
        env = dict(os.environ, LD_LIBRARY_PATH=os.pathsep.join(lib_dirs), DYLD_LIBRARY_PATH=os.pathsep.join(lib_dirs))
        result = subprocess.run([args[0], "-I", *args[1:]], capture_output=True, text=True, env=env)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
