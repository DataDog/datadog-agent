import os
import unittest
from unittest.mock import MagicMock, patch

from invoke.exceptions import Exit

from tasks.new_e2e_tests import (
    _extract_version_from_pipeline_artifacts,
    _path_to_file_url,
    _resolve_local_build,
    _resolve_pipeline_build,
    _resolve_release_build,
    _version_from_msi_filename,
)


class TestVersionFromMsiFilename(unittest.TestCase):
    def test_devel_build(self):
        result = _version_from_msi_filename("datadog-agent-7.75.0-devel.git.59.ac0523a-1-x86_64.msi")
        self.assertEqual(result, ("7.75.0-devel", "7.75.0-devel.git.59.ac0523a-1"))

    def test_stable_build(self):
        result = _version_from_msi_filename("datadog-agent-7.75.0-1-x86_64.msi")
        self.assertEqual(result, ("7.75.0", "7.75.0-1"))

    def test_rc_build(self):
        result = _version_from_msi_filename("datadog-agent-7.76.0-rc.2-1-x86_64.msi")
        self.assertEqual(result, ("7.76.0-rc.2", "7.76.0-rc.2-1"))

    def test_pipeline_build(self):
        result = _version_from_msi_filename("datadog-agent-7.78.0-devel.git.224.602ad25.pipeline.99803655-1-x86_64.msi")
        self.assertEqual(result, ("7.78.0-devel", "7.78.0-devel.git.224.602ad25.pipeline.99803655-1"))

    def test_fips_build(self):
        result = _version_from_msi_filename("datadog-fips-agent-7.75.0-1-x86_64.msi")
        self.assertEqual(result, ("7.75.0", "7.75.0-1"))

    def test_full_s3_path(self):
        result = _version_from_msi_filename("pipelines/A7/12345/datadog-agent-7.75.0-1-x86_64.msi")
        self.assertEqual(result, ("7.75.0", "7.75.0-1"))

    def test_non_msi_file(self):
        self.assertIsNone(_version_from_msi_filename("readme.txt"))

    def test_non_agent_msi(self):
        self.assertIsNone(_version_from_msi_filename("something-else-7.75.0-1-x86_64.msi"))

    def test_amd64_arch(self):
        result = _version_from_msi_filename("datadog-agent-7.75.0-1-amd64.msi")
        self.assertEqual(result, ("7.75.0", "7.75.0-1"))

    def test_no_suffix_version(self):
        result = _version_from_msi_filename("datadog-agent-7.75.0-x86_64.msi")
        self.assertEqual(result, ("7.75.0", "7.75.0"))


class TestExtractVersionFromPipelineArtifacts(unittest.TestCase):
    def test_picks_base_agent_over_fips(self):
        keys = [
            "pipelines/A7/12345/datadog-fips-agent-7.75.0-1-x86_64.msi",
            "pipelines/A7/12345/datadog-agent-7.75.0-1-x86_64.msi",
        ]
        result = _extract_version_from_pipeline_artifacts(keys)
        self.assertEqual(result, ("7.75.0", "7.75.0-1"))

    def test_only_fips_raises(self):
        keys = [
            "pipelines/A7/12345/datadog-fips-agent-7.75.0-1-x86_64.msi",
        ]
        with self.assertRaises(Exit):
            _extract_version_from_pipeline_artifacts(keys)

    def test_empty_list_raises(self):
        with self.assertRaises(Exit):
            _extract_version_from_pipeline_artifacts([])

    def test_non_msi_keys_raises(self):
        keys = [
            "pipelines/A7/12345/datadog-agent-7.75.0-1-windows-amd64.oci.tar",
            "pipelines/A7/12345/some-other-file.zip",
        ]
        with self.assertRaises(Exit):
            _extract_version_from_pipeline_artifacts(keys)

    def test_mixed_keys_finds_msi(self):
        keys = [
            "pipelines/A7/12345/datadog-agent-7.75.0-1-windows-amd64.oci.tar",
            "pipelines/A7/12345/some-other-file.zip",
            "pipelines/A7/12345/datadog-agent-7.75.0-1-x86_64.msi",
        ]
        result = _extract_version_from_pipeline_artifacts(keys)
        self.assertEqual(result, ("7.75.0", "7.75.0-1"))

    def test_devel_pipeline_version(self):
        keys = [
            "pipelines/A7/99803655/datadog-agent-7.78.0-devel.git.224.602ad25.pipeline.99803655-1-x86_64.msi",
        ]
        result = _extract_version_from_pipeline_artifacts(keys)
        self.assertEqual(result, ("7.78.0-devel", "7.78.0-devel.git.224.602ad25.pipeline.99803655-1"))


class TestPathToFileUrl(unittest.TestCase):
    @unittest.skipUnless(os.name == 'nt', "Windows-only test")
    @patch('os.path.abspath', return_value='C:\\omnibus\\pkg\\agent.msi')
    def test_windows_path(self, _mock_abspath):
        result = _path_to_file_url('C:\\omnibus\\pkg\\agent.msi')
        self.assertEqual(result, "file://C:/omnibus/pkg/agent.msi")

    @unittest.skipUnless(os.name == 'posix', "POSIX-only test")
    @patch('os.path.abspath', return_value='/tmp/agent.msi')
    def test_unix_path(self, _mock_abspath):
        result = _path_to_file_url('/tmp/agent.msi')
        self.assertEqual(result, "file:///tmp/agent.msi")


class TestResolveReleaseBuild(unittest.TestCase):
    def test_stable_version_explicit(self):
        env_vars = {}
        _resolve_release_build("STABLE_AGENT", env_vars, version="7.75.0")
        self.assertEqual(env_vars["STABLE_AGENT_ASSERT_VERSION"], "7.75.0")
        self.assertEqual(env_vars["STABLE_AGENT_ASSERT_PACKAGE_VERSION"], "7.75.0-1")
        self.assertEqual(env_vars["STABLE_AGENT_SOURCE_VERSION"], "7.75.0-1")

    def test_rc_version(self):
        env_vars = {}
        _resolve_release_build("STABLE_AGENT", env_vars, version="7.76.0-rc.2")
        self.assertEqual(env_vars["STABLE_AGENT_ASSERT_VERSION"], "7.76.0-rc.2")
        self.assertEqual(env_vars["STABLE_AGENT_ASSERT_PACKAGE_VERSION"], "7.76.0-rc.2-1")
        self.assertEqual(env_vars["STABLE_AGENT_SOURCE_VERSION"], "7.76.0-rc.2-1")

    @patch("tasks.new_e2e_tests.load_release_json")
    def test_default_reads_release_json(self, mock_load):
        mock_load.return_value = {"last_stable": {"7": "7.76.1"}}
        env_vars = {}
        _resolve_release_build("STABLE_AGENT", env_vars)
        self.assertEqual(env_vars["STABLE_AGENT_ASSERT_VERSION"], "7.76.1")
        self.assertEqual(env_vars["STABLE_AGENT_ASSERT_PACKAGE_VERSION"], "7.76.1-1")
        self.assertEqual(env_vars["STABLE_AGENT_SOURCE_VERSION"], "7.76.1-1")

    @patch("tasks.new_e2e_tests.load_release_json", side_effect=Exception("file not found"))
    def test_release_json_fallback(self, _mock_load):
        env_vars = {}
        _resolve_release_build("STABLE_AGENT", env_vars)
        self.assertEqual(env_vars["STABLE_AGENT_ASSERT_VERSION"], "7.75.0")

    def test_custom_prefix(self):
        env_vars = {}
        _resolve_release_build("CURRENT_AGENT", env_vars, version="7.80.0")
        self.assertIn("CURRENT_AGENT_ASSERT_VERSION", env_vars)
        self.assertIn("CURRENT_AGENT_ASSERT_PACKAGE_VERSION", env_vars)
        self.assertIn("CURRENT_AGENT_SOURCE_VERSION", env_vars)
        self.assertNotIn("STABLE_AGENT_ASSERT_VERSION", env_vars)


class TestResolvePipelineBuild(unittest.TestCase):
    @patch("tasks.new_e2e_tests._list_pipeline_msi_files")
    def test_explicit_pipeline_id(self, mock_list):
        mock_list.return_value = [
            "pipelines/A7/12345/datadog-agent-7.75.0-devel.git.10.abc1234-1-x86_64.msi",
        ]
        ctx = MagicMock()
        env_vars = {}
        _resolve_pipeline_build(ctx, "CURRENT_AGENT", env_vars, pipeline_id="12345")

        self.assertEqual(env_vars["CURRENT_AGENT_PIPELINE"], "12345")
        self.assertEqual(env_vars["CURRENT_AGENT_ASSERT_VERSION"], "7.75.0-devel")
        self.assertEqual(env_vars["CURRENT_AGENT_ASSERT_PACKAGE_VERSION"], "7.75.0-devel.git.10.abc1234-1")
        mock_list.assert_called_once_with("12345")

    @patch("tasks.new_e2e_tests._list_pipeline_msi_files")
    @patch("tasks.new_e2e_tests._find_recent_successful_pipeline", return_value="67890")
    def test_auto_detect_pipeline(self, mock_find, mock_list):
        mock_list.return_value = [
            "pipelines/A7/67890/datadog-agent-7.76.0-1-x86_64.msi",
        ]
        ctx = MagicMock()
        env_vars = {}
        _resolve_pipeline_build(ctx, "STABLE_AGENT", env_vars, branch="main")

        self.assertEqual(env_vars["STABLE_AGENT_PIPELINE"], "67890")
        self.assertEqual(env_vars["STABLE_AGENT_ASSERT_VERSION"], "7.76.0")
        mock_find.assert_called_once_with(ctx, "main")
        mock_list.assert_called_once_with("67890")

    @patch("tasks.new_e2e_tests._find_recent_successful_pipeline", return_value=None)
    def test_auto_detect_fails(self, _mock_find):
        ctx = MagicMock()
        env_vars = {}
        with self.assertRaises(Exit):
            _resolve_pipeline_build(ctx, "CURRENT_AGENT", env_vars)

    @patch("tasks.new_e2e_tests._list_pipeline_msi_files")
    def test_no_agent_msi_in_listing(self, mock_list):
        mock_list.return_value = [
            "pipelines/A7/12345/some-other-file.zip",
        ]
        ctx = MagicMock()
        env_vars = {}
        with self.assertRaises(Exit):
            _resolve_pipeline_build(ctx, "CURRENT_AGENT", env_vars, pipeline_id="12345")


class TestResolveLocalBuild(unittest.TestCase):
    @patch("tasks.new_e2e_tests.os.path.isfile", return_value=False)
    @patch("tasks.new_e2e_tests._parse_version_from_msi_filename")
    @patch("tasks.new_e2e_tests._find_local_msi_build")
    @patch(
        "tasks.new_e2e_tests._path_to_file_url", return_value="file://C:/omnibus/pkg/datadog-agent-7.75.0-1-x86_64.msi"
    )
    def test_msi_found_version_parsed(self, _mock_url, mock_find, mock_parse, _mock_isfile):
        mock_find.return_value = "C:\\omnibus\\pkg\\datadog-agent-7.75.0-1-x86_64.msi"
        mock_parse.return_value = ("7.75.0", "7.75.0-1")
        ctx = MagicMock()
        env_vars = {}
        _resolve_local_build(ctx, "CURRENT_AGENT", env_vars)

        self.assertEqual(env_vars["CURRENT_AGENT_MSI_URL"], "file://C:/omnibus/pkg/datadog-agent-7.75.0-1-x86_64.msi")
        self.assertEqual(env_vars["CURRENT_AGENT_ASSERT_VERSION"], "7.75.0")
        self.assertEqual(env_vars["CURRENT_AGENT_ASSERT_PACKAGE_VERSION"], "7.75.0-1")

    @patch("tasks.new_e2e_tests.os.path.isfile", return_value=True)
    @patch("tasks.new_e2e_tests._parse_version_from_msi_filename")
    @patch("tasks.new_e2e_tests._find_local_msi_build")
    @patch("tasks.new_e2e_tests._path_to_file_url")
    def test_msi_with_oci_present(self, mock_url, mock_find, mock_parse, _mock_isfile):
        mock_find.return_value = "C:\\omnibus\\pkg\\datadog-agent-7.75.0-1-x86_64.msi"
        mock_parse.return_value = ("7.75.0", "7.75.0-1")
        mock_url.side_effect = lambda p: f"file://{p.replace(os.sep, '/')}"
        ctx = MagicMock()
        env_vars = {}
        _resolve_local_build(ctx, "CURRENT_AGENT", env_vars)

        self.assertIn("CURRENT_AGENT_OCI_URL", env_vars)

    @patch("tasks.new_e2e_tests.os.path.isfile", return_value=False)
    @patch("tasks.new_e2e_tests._parse_version_from_msi_filename")
    @patch("tasks.new_e2e_tests._find_local_msi_build")
    @patch("tasks.new_e2e_tests._path_to_file_url", return_value="file://C:/omnibus/pkg/agent.msi")
    def test_msi_without_oci(self, _mock_url, mock_find, mock_parse, _mock_isfile):
        mock_find.return_value = "C:\\omnibus\\pkg\\datadog-agent-7.75.0-1-x86_64.msi"
        mock_parse.return_value = ("7.75.0", "7.75.0-1")
        ctx = MagicMock()
        env_vars = {}
        _resolve_local_build(ctx, "CURRENT_AGENT", env_vars)

        self.assertNotIn("CURRENT_AGENT_OCI_URL", env_vars)

    @patch("tasks.new_e2e_tests._find_local_msi_build", return_value=None)
    def test_no_msi_found_no_pkg(self, _mock_find):
        ctx = MagicMock()
        env_vars = {}
        with self.assertRaises(Exit) as cm:
            _resolve_local_build(ctx, "CURRENT_AGENT", env_vars)
        self.assertIn("msi.build", str(cm.exception))

    @patch("tasks.new_e2e_tests._find_local_msi_build", return_value=None)
    def test_no_msi_found_with_pkg(self, _mock_find):
        ctx = MagicMock()
        env_vars = {}
        with self.assertRaises(Exit) as cm:
            _resolve_local_build(ctx, "CURRENT_AGENT", env_vars, pkg="foo")
        self.assertIn("foo", str(cm.exception))
