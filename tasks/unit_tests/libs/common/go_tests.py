import os
import subprocess
import sys
import unittest
from unittest.mock import MagicMock, call, patch

from invoke import Result

from tasks.libs.common.go import _with_pdb_extldflag, go_build
from tasks.libs.common.utils import join_command


class TestGoBuild(unittest.TestCase):
    def setUp(self) -> None:
        super().setUp()
        self.ctx_mock = MagicMock()
        patcher = patch("tasks.libs.common.go.bazel")
        self.bazel_mock = patcher.start()
        self.addCleanup(patcher.stop)
        self.bazel_mock.return_value = subprocess.CompletedProcess(args=(), returncode=0, stdout="", stderr="")

    def assert_bazel_go(self, target, *args, env=None):
        self.bazel_mock.assert_called_once_with(
            "run", target, "--", "-C", os.getcwd(), "build", *args, env=env, capture_stderr=False, ignore_errors=True
        )

    def test_default(self):
        result = go_build(self.ctx_mock, "main.go")

        self.assertEqual(result.exited, 0)
        self.assert_bazel_go("//:go", "-trimpath", "main.go")

    def test_with_gcflags(self):
        result = go_build(self.ctx_mock, "main.go", gcflags="-gcflags=all=-N -l")

        self.assertEqual(result.exited, 0)
        self.assert_bazel_go("//:go", "-gcflags=-gcflags=all=-N -l", "-trimpath", "main.go")

    def test_fips_mode(self):
        result = go_build(self.ctx_mock, "main.go", env={"CGO_ENABLED": "0"}, fips_mode=True)

        self.assertEqual(result.exited, 0)
        self.assert_bazel_go("//:msgo", "-trimpath", "main.go", env={"CGO_ENABLED": "0"})

    @patch.object(sys, "platform", "aix")
    def test_aix(self):
        self.ctx_mock.run.return_value = Result(stdout="", stderr="", exited=0)

        result = go_build(self.ctx_mock, "main.go", gcflags="-gcflags=all=-N -l")

        self.assertEqual(result.exited, 0)
        self.bazel_mock.assert_not_called()
        calls = [call(join_command(["go", "build", "-gcflags=-gcflags=all=-N -l", "-trimpath", "main.go"]), env=None)]
        self.ctx_mock.run.assert_has_calls(calls, any_order=False)

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    @patch.dict("os.environ", {"HOST_UID": "1000", "HOST_GID": "1001"})
    def test_owner_correction(self, mock_chown, mock_exists):
        mock_exists.return_value = True

        result = go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        self.assertEqual(result.exited, 0)
        mock_exists.assert_called_once_with("/tmp/test_binary")
        mock_chown.assert_called_once_with("/tmp/test_binary", 1000, 1001)

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_default_no_owner_correction(self, mock_chown, mock_exists):
        mock_exists.return_value = True

        result = go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        self.assertEqual(result.exited, 0)
        mock_exists.assert_called_once_with("/tmp/test_binary")
        mock_chown.assert_not_called()

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_no_owner_correction_binary_not_exists(self, mock_chown, mock_exists):
        mock_exists.return_value = False

        result = go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        self.assertEqual(result.exited, 0)
        mock_exists.assert_called_once_with("/tmp/test_binary")
        mock_chown.assert_not_called()

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_no_owner_correction_no_bin_path(self, mock_chown, mock_exists):
        result = go_build(self.ctx_mock, "main.go")

        self.assertEqual(result.exited, 0)
        mock_exists.assert_not_called()
        mock_chown.assert_not_called()

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_no_owner_correction_build_failure(self, mock_chown, mock_exists):
        self.bazel_mock.return_value = subprocess.CompletedProcess(args=(), returncode=1, stdout="", stderr="")
        mock_exists.return_value = True

        with self.assertRaises(SystemExit):
            go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        mock_exists.assert_not_called()
        mock_chown.assert_not_called()


class TestWithPDBExtldflag(unittest.TestCase):
    """
    The helper splices `-Wl,--pdb=...` into the existing
    `'-extldflags=...'` group emitted by ``get_build_flags`` — Go's linker
    honors only the last `-extldflags=`, so a clean append would silently
    drop other flags in that group.
    """

    def test_no_existing_ldflags(self):
        out = _with_pdb_extldflag("", "bin/agent/agent.exe")
        self.assertTrue(out.startswith("'-extldflags="))
        self.assertIn(os.path.abspath("bin/agent/agent.exe.pdb"), out)
        self.assertEqual(out.count("-extldflags="), 1)

    def test_existing_ldflags_no_extldflags(self):
        out = _with_pdb_extldflag("-X main.foo=bar", "bin/agent/agent.exe")
        self.assertIn("-X main.foo=bar", out)
        self.assertEqual(out.count("-extldflags="), 1)
        self.assertIn("-Wl,--pdb=", out)

    def test_splices_into_single_quoted_extldflags(self):
        ld = "-X main.foo=bar '-extldflags=-Wl,--version-script=foo.map -Wl,-z,lazy ' "
        out = _with_pdb_extldflag(ld, "bin/agent/agent.exe")
        # Must still have exactly one -extldflags= (Go honors the last).
        self.assertEqual(out.count("-extldflags="), 1)
        # Prior flags preserved inside the group.
        self.assertIn("-Wl,--version-script=foo.map", out)
        self.assertIn("-Wl,-z,lazy", out)
        # Our flag inserted before the closing quote.
        self.assertIn("-Wl,--pdb=", out)
        # Closing quote still present after the splice.
        self.assertIn("' ", out[out.index("-extldflags=") :])

    def test_uses_absolute_path(self):
        # Linker writes the PDB during link, before `go build` copies the
        # .exe to its final location — an absolute path means we don't
        # depend on the linker's CWD.
        out = _with_pdb_extldflag("", "bin/agent/agent.exe")
        self.assertIn(os.path.abspath("bin/agent/agent.exe.pdb"), out)
