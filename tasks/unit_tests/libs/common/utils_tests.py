import os
import shutil
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from tasks.libs.common.utils import (
    RTLOADER_HEADER_NAME,
    RTLOADER_LIB_NAME,
    get_build_flags,
    get_rtloader_paths,
    link_or_copy,
    running_in_ci,
    running_in_github_actions,
    running_in_gitlab_ci,
    running_in_pre_commit,
    running_in_pyapp,
)


class TestRunningIn(unittest.TestCase):
    def test_running_in(self):
        parameters = [
            ("PRE_COMMIT", "1", True, running_in_pre_commit),
            ("PRE_COMMIT", "", False, running_in_pre_commit),
            ("PYAPP", "1", True, running_in_pyapp),
            ("PYAPP", "", False, running_in_pyapp),
            ("GITLAB_CI", "true", True, running_in_gitlab_ci),
            ("GITLAB_CI", "", False, running_in_gitlab_ci),
            ("GITHUB_ACTIONS", "true", True, running_in_github_actions),
            ("GITHUB_ACTIONS", "", False, running_in_github_actions),
            ("GITHUB_ACTIONS", "true", True, running_in_ci),
            ("GITLAB_CI", "true", True, running_in_ci),
            ("GITHUB_ACTIONS", "false", False, running_in_ci),
            ("GITLAB_CI", "false", False, running_in_ci),
        ]

        for env_var, value, expected, func in parameters:
            with self.subTest(env_var=env_var, value=value, expected_value=expected):
                with mock.patch.dict(os.environ, {env_var: value}, clear=True):
                    self.assertEqual(expected, func())


class TestLinkOrCopy(unittest.TestCase):
    def setUp(self):
        self._tmpdir = Path(tempfile.mkdtemp(prefix="test-link-or-copy"))
        self.src = self._tmpdir / "src.txt"
        self.dst = self._tmpdir / "dst.txt"
        self.src.write_text("new")

    def tearDown(self):
        shutil.rmtree(self._tmpdir)

    def test_links_or_copies(self):
        link_or_copy(self.src, self.dst)
        self.assertEqual(self.dst.read_text(), "new")

    def test_removes_old_dst(self):
        self.dst.write_text("old")
        link_or_copy(self.src, self.dst)
        self.assertEqual(self.dst.read_text(), "new")

    @mock.patch("tasks.libs.common.utils.shutil.copy2")
    @mock.patch.object(Path, "hardlink_to", side_effect=OSError)
    @mock.patch.object(Path, "symlink_to", side_effect=OSError)
    def test_tries_methods_in_order(self, symlink_to, hardlink_to, copy2):
        in_order = mock.Mock()
        in_order.attach_mock(symlink_to, "symlink_to")
        in_order.attach_mock(hardlink_to, "hardlink_to")
        in_order.attach_mock(copy2, "copy2")

        link_or_copy(self.src, self.dst)

        in_order.assert_has_calls(
            (
                mock.call.symlink_to(Path("src.txt")),
                mock.call.hardlink_to(self.src),
                mock.call.symlink_to(self.src),
                mock.call.copy2(self.src, self.dst),
            )
        )

    @mock.patch("tasks.libs.common.utils.shutil.copy2", side_effect=OSError)
    @mock.patch.object(Path, "hardlink_to", side_effect=NotImplementedError)
    @mock.patch.object(Path, "symlink_to", side_effect=NotImplementedError)
    def test_propagates_last_error(self, symlink_to, hardlink_to, copy2):
        with self.assertRaises(OSError):
            link_or_copy(self.src, self.dst)


class TestGetRtloaderPaths(unittest.TestCase):
    def setUp(self):
        self._tmpdir = Path(tempfile.mkdtemp(prefix="test-rtloader-paths"))

    def tearDown(self):
        shutil.rmtree(self._tmpdir)

    def test_finds_bazel_install_under_embedded_dir(self):
        lib_dir = self._tmpdir / "dev" / "embedded" / "lib"
        include_dir = self._tmpdir / "dev" / "embedded" / "include"
        lib_dir.mkdir(parents=True)
        include_dir.mkdir(parents=True)
        (lib_dir / RTLOADER_LIB_NAME).touch()
        (include_dir / RTLOADER_HEADER_NAME).touch()

        rtloader_lib, rtloader_headers, rtloader_common_headers = get_rtloader_paths(embedded_path=self._tmpdir / "dev")

        self.assertEqual(rtloader_lib, [str(lib_dir)])
        self.assertEqual(rtloader_headers, str(include_dir))
        self.assertEqual(rtloader_common_headers, "")

    def test_prefers_legacy_install_over_embedded_dir(self):
        legacy_lib_dir = self._tmpdir / "dev" / "lib"
        embedded_lib_dir = self._tmpdir / "dev" / "embedded" / "lib"
        legacy_lib_dir.mkdir(parents=True)
        embedded_lib_dir.mkdir(parents=True)
        (legacy_lib_dir / RTLOADER_LIB_NAME).touch()
        (embedded_lib_dir / RTLOADER_LIB_NAME).touch()

        rtloader_lib, _, _ = get_rtloader_paths(embedded_path=self._tmpdir / "dev")

        self.assertEqual(rtloader_lib, [str(legacy_lib_dir)])

    def test_finds_headers_under_embedded_dir_when_lib_is_in_legacy_dir(self):
        lib_dir = self._tmpdir / "dev" / "lib"
        include_dir = self._tmpdir / "dev" / "embedded" / "include"
        lib_dir.mkdir(parents=True)
        include_dir.mkdir(parents=True)
        (lib_dir / RTLOADER_LIB_NAME).touch()
        (include_dir / RTLOADER_HEADER_NAME).touch()

        rtloader_lib, rtloader_headers, _ = get_rtloader_paths(embedded_path=self._tmpdir / "dev")

        self.assertEqual(rtloader_lib, [str(lib_dir)])
        self.assertEqual(rtloader_headers, str(include_dir))


class TestGetBuildFlags(unittest.TestCase):
    @mock.patch("tasks.libs.common.utils.get_version_ldflags", return_value="")
    @mock.patch("tasks.libs.common.utils.get_rtloader_paths", return_value=(["/dev/embedded/lib"], "", ""))
    def test_infers_python_home_from_bazel_rtloader_path(self, _get_rtloader_paths, _get_version_ldflags):
        ldflags, _, _ = get_build_flags(mock.Mock(), embedded_path="/dev", include_python=True)

        self.assertIn("python.pythonHome3=/dev/embedded", ldflags.replace("\\", "/"))

    @mock.patch("tasks.libs.common.utils.get_version_ldflags", return_value="")
    @mock.patch("tasks.libs.common.utils.get_rtloader_paths", return_value=(["/external/embedded/lib"], "", ""))
    def test_infers_python_home_from_selected_rtloader_root_path(self, _get_rtloader_paths, _get_version_ldflags):
        ldflags, _, _ = get_build_flags(mock.Mock(), embedded_path="/dev", rtloader_root="/external", include_python=True)

        self.assertIn("python.pythonHome3=/external/embedded", ldflags.replace("\\", "/"))

    @mock.patch("tasks.libs.common.utils.get_version_ldflags", return_value="")
    @mock.patch("tasks.libs.common.utils.get_rtloader_paths", return_value=(["/dev/lib"], "", ""))
    def test_does_not_infer_python_home_from_legacy_rtloader_path(self, _get_rtloader_paths, _get_version_ldflags):
        ldflags, _, _ = get_build_flags(mock.Mock(), embedded_path="/dev", include_python=True)

        self.assertNotIn("python.pythonHome3", ldflags)

    @mock.patch("tasks.libs.common.utils.get_version_ldflags", return_value="")
    @mock.patch(
        "tasks.libs.common.utils.get_rtloader_paths", return_value=(["/external/lib", "/dev/embedded/lib"], "", "")
    )
    def test_does_not_infer_python_home_when_selected_rtloader_is_legacy(
        self, _get_rtloader_paths, _get_version_ldflags
    ):
        # The selected (first) rtloader is a legacy root; a stale embedded lib from a prior
        # Bazel build must not override the Python home for the rtloader actually linked.
        ldflags, _, _ = get_build_flags(mock.Mock(), embedded_path="/dev", rtloader_root="/external", include_python=True)

        self.assertNotIn("python.pythonHome3", ldflags)

    @mock.patch("tasks.libs.common.utils.get_version_ldflags", return_value="")
    @mock.patch("tasks.libs.common.utils.get_rtloader_paths", return_value=(["/dev/lib", "/dev/embedded/lib"], "", ""))
    @mock.patch("tasks.libs.common.utils.sys.platform", "darwin")
    @mock.patch("tasks.libs.common.utils.get_xcode_version", return_value="15.0")
    def test_uses_external_linker_for_multiple_rtloader_rpaths_on_macos(
        self, _get_xcode_version, _get_rtloader_paths, _get_version_ldflags
    ):
        ldflags, _, _ = get_build_flags(mock.Mock(), embedded_path="/dev", include_python=True)

        self.assertIn("-Wl,-rpath,/dev/lib", ldflags)
        self.assertIn("-Wl,-rpath,/dev/embedded/lib", ldflags)
