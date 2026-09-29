"""Integration tests: bake known-broken or known-healthy manifests from real
compiled fixtures and verify the healthcheck reaches the right verdict.

Each test wires a tiny C lib + a consumer .so whose DT_NEEDED references it,
and builds a synthetic Manifest directly from the real fixture files. py_test's
runtime CWD is its own runfiles directory, not the execroot, so a
Starlark-written manifest (execroot-relative paths, correct for the
production build-action path) can't be read directly here; building the
Manifest in Python sidesteps that entirely.
"""

import os
import sys
import unittest

import linux
from manifest import Manifest

_INSTALL_PREFIX = "/opt/datadog-agent"

_LIBBROKEN_ABSOLUTE, _LIBBROKEN_BOGUS, _LIBBROKEN_ORIGIN, _LIBDUMMYBAD = (os.path.abspath(p) for p in sys.argv[1:5])


class IntegrationTest(unittest.TestCase):
    def test_missing_dep_is_caught(self):
        # libbroken.so is shipped without libdummybad.so.
        manifest = Manifest(files={"embedded/bin/libbroken.so": _LIBBROKEN_ABSOLUTE}, symlinks={})
        failures = linux.check(manifest, _INSTALL_PREFIX)
        self.assertEqual(len(failures), 1, failures)
        self.assertEqual(next(iter(failures)).dep_name, "libdummybad.so")

    def test_absolute_rpath_passes(self):
        # Both libs present, RPATH matches the configured install prefix.
        manifest = Manifest(
            files={
                "embedded/bin/libbroken.so": _LIBBROKEN_ABSOLUTE,
                "embedded/lib/libdummybad.so": _LIBDUMMYBAD,
            },
            symlinks={},
        )
        failures = linux.check(manifest, _INSTALL_PREFIX)
        self.assertEqual(failures, set(), failures)

    def test_bogus_rpath_is_caught(self):
        # Both libs bundled, but RPATH points to a non-existent location.
        manifest = Manifest(
            files={
                "embedded/bin/libbroken.so": _LIBBROKEN_BOGUS,
                "embedded/lib/libdummybad.so": _LIBDUMMYBAD,
            },
            symlinks={},
        )
        failures = linux.check(manifest, _INSTALL_PREFIX)
        self.assertEqual(len(failures), 1, failures)
        self.assertEqual(next(iter(failures)).dep_name, "libdummybad.so")

    def test_origin_rpath_is_rejected(self):
        # Both libs bundled and correctly laid out, but RPATH is $ORIGIN-relative.
        manifest = Manifest(
            files={
                "embedded/bin/libbroken.so": _LIBBROKEN_ORIGIN,
                "embedded/lib/libdummybad.so": _LIBDUMMYBAD,
            },
            symlinks={},
        )
        failures = linux.check(manifest, _INSTALL_PREFIX)
        dep_names = {f.dep_name for f in failures}
        self.assertIn("$ORIGIN in rpath", dep_names, failures)
        # With the only rpath rejected, no candidate directory remains either,
        # so the dependency that would otherwise resolve there also fails.
        self.assertIn("libdummybad.so", dep_names, failures)


if __name__ == "__main__":
    unittest.main(argv=sys.argv[:1])
