import os
import unittest
from unittest.mock import patch

from tasks.agent import add_rc_x509_cgo_flags, install_rc_x509_linux_build_files


class TestRCX509BuildInputs(unittest.TestCase):
    @patch("tasks.agent.bazel")
    def test_installs_below_an_embedded_prefix_parent(self, bazel):
        embedded_path = os.path.join(os.sep, "tmp", "agent", "embedded")

        native_embedded_path = install_rc_x509_linux_build_files(embedded_path)

        self.assertEqual(native_embedded_path, embedded_path)
        bazel.assert_called_once_with(
            "run",
            "@libdd_rc_source//:install_rc_x509_ffi_linux_build_files",
            "--",
            f"--destdir={os.path.join(os.sep, 'tmp', 'agent')}",
        )

    @patch("tasks.agent.bazel")
    def test_installs_below_a_development_root(self, bazel):
        development_root = os.path.join(os.sep, "tmp", "agent", "dev")

        native_embedded_path = install_rc_x509_linux_build_files(development_root)

        self.assertEqual(native_embedded_path, os.path.join(development_root, "embedded"))
        bazel.assert_called_once_with(
            "run",
            "@libdd_rc_source//:install_rc_x509_ffi_linux_build_files",
            "--",
            f"--destdir={development_root}",
        )

    def test_adds_native_paths_without_discarding_existing_cgo_flags(self):
        env = {
            "CGO_CFLAGS": "-Werror",
            "CGO_LDFLAGS": "-L/existing/lib",
        }
        native_embedded_path = os.path.join(os.sep, "tmp", "agent", "embedded")

        add_rc_x509_cgo_flags(env, native_embedded_path)

        self.assertEqual(env["CGO_CFLAGS"], f"-Werror -I{native_embedded_path}/include")
        self.assertEqual(env["CGO_LDFLAGS"], f"-L/existing/lib -L{native_embedded_path}/lib")


if __name__ == "__main__":
    unittest.main()
