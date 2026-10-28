import os
import unittest
from unittest.mock import patch

from invoke.exceptions import Exit

from tasks.agent import add_rc_x509_cgo_flags, install_rc_x509_build_files, needs_rc_x509_build_files
from tasks.flavor import AgentFlavor


class TestRCX509BuildInputs(unittest.TestCase):
    @patch("tasks.agent.bazel")
    def test_installs_below_an_embedded_prefix_parent(self, bazel):
        embedded_path = os.path.join(os.sep, "tmp", "agent", "embedded")

        native_embedded_path = install_rc_x509_build_files(embedded_path, "linux", AgentFlavor.base)

        self.assertEqual(native_embedded_path, embedded_path)
        bazel.assert_called_once_with(
            "run",
            "--//packages/agent:flavor=base",
            "@libdd_rc_source//:install_rc_x509_ffi_linux_build_files",
            "--",
            f"--destdir={os.path.join(os.sep, 'tmp', 'agent')}",
        )

    @patch("tasks.agent.bazel")
    def test_installs_below_a_development_root(self, bazel):
        development_root = os.path.join(os.sep, "tmp", "agent", "dev")

        native_embedded_path = install_rc_x509_build_files(development_root, "linux-arm64", AgentFlavor.base)

        self.assertEqual(native_embedded_path, os.path.join(development_root, "embedded"))
        bazel.assert_called_once_with(
            "run",
            "--//packages/agent:flavor=base",
            "@libdd_rc_source//:install_rc_x509_ffi_linux_build_files",
            "--",
            f"--destdir={development_root}",
        )

    @patch("tasks.agent.bazel")
    def test_installs_macos_build_files(self, bazel):
        embedded_path = os.path.join(os.sep, "tmp", "agent", "embedded")

        native_embedded_path = install_rc_x509_build_files(embedded_path, "darwin", AgentFlavor.base)

        self.assertEqual(native_embedded_path, embedded_path)
        bazel.assert_called_once_with(
            "run",
            "--//packages/agent:flavor=base",
            "@libdd_rc_source//:install_rc_x509_ffi_macos_build_files",
            "--",
            f"--destdir={os.path.join(os.sep, 'tmp', 'agent')}",
        )

    @patch("tasks.agent.bazel")
    def test_builds_fips_inputs_for_the_fips_flavor(self, bazel):
        embedded_path = os.path.join(os.sep, "tmp", "agent", "embedded")

        install_rc_x509_build_files(embedded_path, "linux", AgentFlavor.fips)

        bazel.assert_called_once_with(
            "run",
            "--//packages/agent:flavor=fips",
            "@libdd_rc_source//:install_rc_x509_ffi_linux_build_files",
            "--",
            f"--destdir={os.path.join(os.sep, 'tmp', 'agent')}",
        )

    @patch("tasks.agent.bazel")
    def test_rejects_unsupported_platform(self, bazel):
        with self.assertRaisesRegex(Exit, "not supported on win32"):
            install_rc_x509_build_files(os.path.join(os.sep, "tmp", "agent", "embedded"), "win32", AgentFlavor.base)
        bazel.assert_not_called()

    def test_adds_linux_native_paths_without_discarding_existing_cgo_flags(self):
        env = {
            "CGO_CFLAGS": "-Werror",
            "CGO_LDFLAGS": "-L/existing/lib",
        }
        native_embedded_path = os.path.join(os.sep, "tmp", "agent", "embedded")

        add_rc_x509_cgo_flags(env, native_embedded_path, "linux")

        self.assertEqual(env["CGO_CFLAGS"], f"-Werror -I{native_embedded_path}/include")
        self.assertEqual(env["CGO_LDFLAGS"], f"-L/existing/lib -L{native_embedded_path}/lib")

    def test_adds_macos_native_paths_and_runtime_search_path(self):
        env = {}
        native_embedded_path = os.path.join(os.sep, "tmp", "agent", "embedded")

        add_rc_x509_cgo_flags(env, native_embedded_path, "darwin")

        self.assertEqual(env["CGO_CFLAGS"], f" -I{native_embedded_path}/include")
        self.assertEqual(
            env["CGO_LDFLAGS"],
            f" -L{native_embedded_path}/lib -Wl,-rpath,{native_embedded_path}/lib",
        )

    def test_needs_build_files_when_rc_x509_tag_is_set(self):
        self.assertTrue(needs_rc_x509_build_files({"rc_x509", "zlib"}, "linux", enable_bazel=False))
        self.assertTrue(needs_rc_x509_build_files({"rc_x509"}, "darwin", enable_bazel=False))

    def test_skips_build_files_without_rc_x509_tag(self):
        # e.g. --build-exclude=rc_x509, or the IoT flavor's tag set
        self.assertFalse(needs_rc_x509_build_files({"zlib"}, "linux", enable_bazel=False))

    def test_skips_build_files_on_unsupported_platforms(self):
        self.assertFalse(needs_rc_x509_build_files({"rc_x509"}, "win32", enable_bazel=False))

    def test_skips_build_files_for_bazel_builds(self):
        self.assertFalse(needs_rc_x509_build_files({"rc_x509"}, "linux", enable_bazel=True))


if __name__ == "__main__":
    unittest.main()
