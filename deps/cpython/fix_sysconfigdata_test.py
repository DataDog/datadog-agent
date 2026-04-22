"""Tests for fix_sysconfigdata.py."""

import importlib.util
import os
import tempfile
import textwrap
import unittest

import fix_sysconfigdata

# The sandbox install prefix used in the fixture below, matching what
# $BUILD_TMPDIR/$INSTALL_PREFIX would expand to during the Bazel build.
_SANDBOX_PREFIX = "/execroot/_main/bazel-out/arch/bin/external/+repo+cpython/python_unix.build_tmpdir/python_unix"

# A minimal _sysconfigdata__*.py fixture that exercises the fix-up rules.
# Paths under _SANDBOX_PREFIX represent fields recorded relative to the Python install root
_FIXTURE = textwrap.dedent(f"""\
    build_time_vars = {{
        "CC": "/execroot/_main/external/+toolchain/bin/gcc",
        "AR": "/execroot/_main/external/+toolchain/bin/ar",
        "CFLAGS": "-O2 -nostdinc -isystem /execroot/_main/sysroot/include",
        "LDFLAGS": "-Wl,-rpath,/bazel-out/k8-opt/bin",
        "LDSHARED": "/execroot/_main/bin/gcc -shared -Wl,-z,relro -B/execroot/_main/bin -L/bazel-out/k8-opt/lib",
        "CONFIGURED_CC": "--cc=/execroot/_main/bin/gcc -O2",
        "CONFIG_ARGS": "  '--enable-ipv6' 'LDFLAGS=-Wl,-rpath,/execroot/_main/lib'",
        "PY_CFLAGS": "-isystem/execroot/_main/external/gcc_toolchain/lib/gcc/aarch64-unknown-linux-gnu/12.3.0/include -O2",
        "VERSION": "3.13.0",
        "LIBDIR": "{_SANDBOX_PREFIX}/lib",
        "INCLUDEPY": "{_SANDBOX_PREFIX}/include/python3.13",
        "prefix": "{_SANDBOX_PREFIX}",
    }}
""")

_PREFIX = "##PREFIX##"


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
            fix_sysconfigdata.fix_file(path, install_prefix=_PREFIX, sandbox_prefix=_SANDBOX_PREFIX)
            build_time_vars = _load_fixed(path)

        # Pure install-dir paths are rewritten using the placeholder prefix
        self.assertEqual(build_time_vars["LIBDIR"], _PREFIX + "/lib")
        self.assertEqual(build_time_vars["INCLUDEPY"], _PREFIX + "/include/python3.13")
        self.assertEqual(build_time_vars["prefix"], _PREFIX)

        # FLAGS_TO_CLEAR: wiped entirely
        self.assertEqual(build_time_vars["CFLAGS"], "")
        self.assertEqual(build_time_vars["LDFLAGS"], "")

        # Tool paths reduced to basename (via _fix_tool_path)
        self.assertEqual(build_time_vars["CC"], "gcc")
        self.assertEqual(build_time_vars["AR"], "ar")

        # --flag=/path/to/tool: the '=' boundary is respected so
        # the flag prefix is preserved and only the path portion is replaced.
        self.assertEqual(build_time_vars["CONFIGURED_CC"], "--cc=gcc -O2")

        # Non-string values pass through unchanged
        self.assertEqual(build_time_vars["VERSION"], "3.13.0")

        # bazel-out paths in LDSHARED are collapsed:
        # -L/bazel-out/k8-opt/lib  -> -L##PREFIX##/lib  (known suffix)
        # -B/execroot/... is not bazel-out so left alone
        self.assertEqual(build_time_vars["LDSHARED"], "gcc -shared -Wl,-z,relro -B/execroot/_main/bin -L##PREFIX##/lib")


if __name__ == "__main__":
    unittest.main()
