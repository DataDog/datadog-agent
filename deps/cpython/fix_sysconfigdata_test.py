"""Tests for fix_sysconfigdata.py."""

import importlib.util
import os
import tempfile
import textwrap
import unittest

import fix_sysconfigdata

# A minimal _sysconfigdata__*.py fixture that exercises the fix-up rules:
_FIXTURE = textwrap.dedent("""\
    build_time_vars = {
        "CC": "/execroot/_main/external/+toolchain/bin/gcc",
        "AR": "/execroot/_main/external/+toolchain/bin/ar",
        "CFLAGS": "-O2 -nostdinc -isystem /execroot/_main/sysroot/include",
        "LDFLAGS": "-Wl,-rpath,/bazel-out/k8-opt/bin",
        "LDSHARED": "/execroot/_main/bin/gcc -shared -Wl,-z,relro -B/execroot/_main/bin -L/bazel-out/k8-opt/lib",
        "CONFIGURED_CC": "--cc=/execroot/_main/bin/gcc -O2",
        "CONFIG_ARGS": "  '--enable-ipv6' 'LDFLAGS=-Wl,-rpath,/execroot/_main/lib'",
        "PY_CFLAGS": "-isystem/execroot/_main/external/gcc_toolchain/lib/gcc/aarch64-unknown-linux-gnu/12.3.0/include -O2",
        "VERSION": "3.13.0",
    }
""")


def _load_fixed(path):
    """Import a patched sysconfigdata file and return its build_time_vars."""
    # https://docs.python.org/3/library/importlib.html#importing-a-source-file-directly
    spec = importlib.util.spec_from_file_location("_sysconfigdata_test", path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod.build_time_vars


class TestFixSysconfigdata(unittest.TestCase):
    def test_fix_file(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            path = os.path.join(tmpdir, "_sysconfigdata__test.py")
            with open(path, "w") as f:
                f.write(_FIXTURE)
            fix_sysconfigdata.fix_file(path)
            build_time_vars = _load_fixed(path)

        # FLAGS_TO_CLEAR: wiped entirely
        self.assertEqual(build_time_vars["CFLAGS"], "")
        self.assertEqual(build_time_vars["LDFLAGS"], "")

        # Tool paths reduced to basename
        self.assertEqual(build_time_vars["CC"], "gcc")
        self.assertEqual(build_time_vars["AR"], "ar")

        # Compound values: tool basename kept, useful flags kept, sandbox tokens stripped.
        self.assertEqual(build_time_vars["LDSHARED"], "gcc -shared -Wl,-z,relro")
        self.assertEqual(build_time_vars["PY_CFLAGS"], "-O2")

        # --flag=/path/to/tool: the '=' boundary is respected so
        # the flag prefix is preserved and only the path portion is replaced.
        self.assertEqual(build_time_vars["CONFIGURED_CC"], "--cc=gcc -O2")

        # Single-quoted shell args containing Bazel paths are dropped as whole tokens;
        # args without Bazel paths are preserved.
        self.assertEqual(build_time_vars["CONFIG_ARGS"], "--enable-ipv6")

        # Non-string values pass through unchanged
        self.assertEqual(build_time_vars["VERSION"], "3.13.0")


if __name__ == "__main__":
    unittest.main()
