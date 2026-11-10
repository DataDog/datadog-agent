import os
import sys
import unittest
from unittest.mock import MagicMock, patch

from tasks.flavor import FipsBackend
from tasks.libs.common.go import _with_pdb_extldflag, go_build
from tasks.libs.common.utils import join_command


class TestGoBuild(unittest.TestCase):
    def setUp(self) -> None:
        super().setUp()
        self.ctx_mock = MagicMock()
        self.bazel_mock = self.enterContext(patch("tasks.libs.common.go.bazel", return_value=""))
        self.which_mock = self.enterContext(
            patch("tasks.libs.common.go.shutil.which", return_value="/usr/bin/bazelisk")
        )

    def test_default(self):
        go_build(self.ctx_mock, "main.go")

        self.bazel_mock.assert_called_once_with(
            "run",
            "@rules_go//go",
            "--",
            "-C",
            os.getcwd(),
            "build",
            "-trimpath",
            "main.go",
            capture_stderr=False,
            env={},
        )

    def test_with_gcflags(self):
        go_build(self.ctx_mock, "main.go", gcflags="-gcflags=all=-N -l")

        self.bazel_mock.assert_called_once_with(
            "run",
            "@rules_go//go",
            "--",
            "-C",
            os.getcwd(),
            "build",
            "-gcflags=-gcflags=all=-N -l",
            "-trimpath",
            "main.go",
            capture_stderr=False,
            env={},
        )

    def test_boringcrypto(self):
        go_build(self.ctx_mock, "main.go", env={"CGO_ENABLED": "1"}, fips_backend=FipsBackend.boringcrypto)

        self.bazel_mock.assert_called_once_with(
            "run",
            "@rules_go//go",
            "--",
            "-C",
            os.getcwd(),
            "build",
            "-trimpath",
            "main.go",
            capture_stderr=False,
            env={"CGO_ENABLED": "1", "GOEXPERIMENT": "boringcrypto"},
        )

    @patch.dict("os.environ", {"GOOS": "linux"})
    def test_msgo_linux(self):
        go_build(self.ctx_mock, "main.go", env={"CGO_ENABLED": "0"}, fips_backend=FipsBackend.msgo)

        self.bazel_mock.assert_called_once_with(
            "run",
            "//:msgo",
            "--",
            "-C",
            os.getcwd(),
            "build",
            "-trimpath",
            "main.go",
            capture_stderr=False,
            env={"CGO_ENABLED": "0", "GOEXPERIMENT": "ms_nocgo_opensslcrypto"},
        )

    @patch.dict("os.environ", {"GOOS": "windows"})
    def test_msgo_windows(self):
        go_build(self.ctx_mock, "main.go", env={"CGO_ENABLED": "1"}, fips_backend=FipsBackend.msgo)

        self.bazel_mock.assert_called_once_with(
            "run",
            "//:msgo",
            "--",
            "-C",
            os.getcwd(),
            "build",
            "-trimpath",
            "main.go",
            capture_stderr=False,
            env={"CGO_ENABLED": "1"},
        )

    def test_without_bazel(self):
        self.which_mock.return_value = None

        go_build(self.ctx_mock, "main.go", gcflags="-gcflags=all=-N -l")

        self.bazel_mock.assert_not_called()
        self.ctx_mock.run.assert_called_once_with(
            join_command(["go", "build", "-gcflags=-gcflags=all=-N -l", "-trimpath", "main.go"]), env={}
        )

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    @patch.dict("os.environ", {"HOST_UID": "1000", "HOST_GID": "1001"})
    def test_owner_correction(self, mock_chown, mock_exists):
        mock_exists.return_value = True

        go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        mock_exists.assert_called_once_with("/tmp/test_binary")
        mock_chown.assert_called_once_with("/tmp/test_binary", 1000, 1001)

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_default_no_owner_correction(self, mock_chown, mock_exists):
        mock_exists.return_value = True

        go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        mock_exists.assert_called_once_with("/tmp/test_binary")
        mock_chown.assert_not_called()

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_no_owner_correction_binary_not_exists(self, mock_chown, mock_exists):
        mock_exists.return_value = False

        go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        mock_exists.assert_called_once_with("/tmp/test_binary")
        mock_chown.assert_not_called()

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_no_owner_correction_no_bin_path(self, mock_chown, mock_exists):
        go_build(self.ctx_mock, "main.go")

        mock_exists.assert_not_called()
        mock_chown.assert_not_called()

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_no_owner_correction_build_failure(self, mock_chown, mock_exists):
        self.bazel_mock.side_effect = SystemExit(1)
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
