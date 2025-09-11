import sys
import unittest
from unittest.mock import MagicMock, call, patch

from invoke import Result

from tasks.libs.common.go import go_build


class TestGoBuild(unittest.TestCase):
    def setUp(self) -> None:
        super().setUp()
        self.ctx_mock = MagicMock()

    def test_default(self):
        self.ctx_mock.run.return_value = Result(stdout="", stderr="", exited=0)

        result = go_build(self.ctx_mock, "main.go")

        self.assertEqual(result.exited, 0)

        calls = [call("go build -trimpath main.go", env=None)]
        self.ctx_mock.run.assert_has_calls(calls, any_order=False)

    def test_with_gcflags(self):
        self.ctx_mock.run.return_value = Result(stdout="", stderr="", exited=0)

        result = go_build(self.ctx_mock, "main.go", gcflags="-gcflags=all=-N -l")

        self.assertEqual(result.exited, 0)
        calls = [call("go build -gcflags=\"-gcflags=all=-N -l\" -trimpath main.go", env=None)]
        self.ctx_mock.run.assert_has_calls(calls, any_order=False)

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    @patch.dict("os.environ", {"HOST_UID": "1000", "HOST_GID": "1001"})
    def test_owner_correction(self, mock_chown, mock_exists):
        self.ctx_mock.run.return_value = Result(stdout="", stderr="", exited=0)
        mock_exists.return_value = True

        result = go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        self.assertEqual(result.exited, 0)
        mock_exists.assert_called_once_with("/tmp/test_binary")
        mock_chown.assert_called_once_with("/tmp/test_binary", 1000, 1001)

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_default_no_owner_correction(self, mock_chown, mock_exists):
        self.ctx_mock.run.return_value = Result(stdout="", stderr="", exited=0)
        mock_exists.return_value = True

        result = go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        self.assertEqual(result.exited, 0)
        mock_exists.assert_called_once_with("/tmp/test_binary")
        mock_chown.assert_not_called()

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_no_owner_correction_binary_not_exists(self, mock_chown, mock_exists):
        self.ctx_mock.run.return_value = Result(stdout="", stderr="", exited=0)
        mock_exists.return_value = False

        result = go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        self.assertEqual(result.exited, 0)
        mock_exists.assert_called_once_with("/tmp/test_binary")
        mock_chown.assert_not_called()

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_no_owner_correction_no_bin_path(self, mock_chown, mock_exists):
        self.ctx_mock.run.return_value = Result(stdout="", stderr="", exited=0)

        result = go_build(self.ctx_mock, "main.go")

        self.assertEqual(result.exited, 0)
        mock_exists.assert_not_called()
        mock_chown.assert_not_called()

    @unittest.skipIf(sys.platform == "win32", "os.chown not available on Windows")
    @patch("os.path.exists")
    @patch("os.chown")
    def test_no_owner_correction_build_failure(self, mock_chown, mock_exists):
        self.ctx_mock.run.return_value = Result(stdout="", stderr="", exited=1)
        mock_exists.return_value = True

        result = go_build(self.ctx_mock, "main.go", bin_path="/tmp/test_binary")

        self.assertEqual(result.exited, 1)
        mock_exists.assert_not_called()
        mock_chown.assert_not_called()
