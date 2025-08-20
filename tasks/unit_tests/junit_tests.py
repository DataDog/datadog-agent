import shutil
import unittest
from pathlib import Path
from subprocess import CalledProcessError
from unittest.mock import patch

import tasks.libs.common.junit_upload_core as junit
from tasks.libs.owners.parsing import read_owners


class TestFindTarball(unittest.TestCase):
    def test_tarball_in_folder(self):
        tarball_in_folder = "./tasks/unit_tests/testdata/secret.tar.gz"
        self.assertEqual(junit.find_tarball(tarball_in_folder), f"{tarball_in_folder}/secret.tar.gz")

    def test_tarball_in_folder_not_found(self):
        tarball_in_folder = "./tasks/unit_tests/testdata/go_mod_formatter"
        self.assertEqual(junit.find_tarball(tarball_in_folder), f"{tarball_in_folder}/junit.tar.gz")


class TestReadAdditionalTags(unittest.TestCase):
    def test_with_tags(self):
        valid_tags = Path("./tasks/unit_tests/testdata/secret.tar.gz")
        self.assertEqual(
            junit.read_additional_tags(valid_tags),
            [
                "--tags",
                "ci.job.name:kitchen_windows_installer_agent-a6",
                "--tags",
                "arch:x86_64",
                "--tags",
                "os:windows",
                "upload_option.os_version_from_name",
            ],
        )

    def test_without_tags(self):
        invalid_tags = Path("./tasks/unit_tests/testdata")
        self.assertEqual(len(junit.read_additional_tags(invalid_tags)), 0)


class TestSplitJUnitXML(unittest.TestCase):
    def tearDown(self) -> None:
        p = Path("./tasks/unit_tests/testdata/secret.tar.gz")
        for dir in p.iterdir():
            if dir.is_dir():
                shutil.rmtree(dir, ignore_errors=True)

    def test_without_split(self):
        xml_file = Path("./tasks/unit_tests/testdata/secret.tar.gz/bedroom-rspec-win2016-azure-x86_64.xml")
        owners = read_owners(".github/CODEOWNERS")
        self.assertEqual(junit.split_junitxml(xml_file.parent, xml_file, owners, {}, {}), 1)
        generated_folder = xml_file.parent / "windows-products_base"
        self.assertTrue(generated_folder.exists())

    def test_with_split(self):
        xml_file = Path("./tasks/unit_tests/testdata/secret.tar.gz/-go-src-datadog-agent-junit-out-base.xml")
        owners = read_owners(".github/CODEOWNERS")
        self.assertEqual(junit.split_junitxml(xml_file.parent, xml_file, owners, {}, {}), 27)


class TestGroupPerTag(unittest.TestCase):
    def test_default_e2e(self):
        test_dir = Path("./tasks/unit_tests/testdata/to_group")
        grouped = junit.group_per_tags(test_dir, [])
        self.assertIn("default", grouped)
        self.assertCountEqual([f"{str(test_dir)}/onepiece", f"{str(test_dir)}/dragonball"], grouped["default"])
        self.assertIn("e2e", grouped)
        self.assertEqual([f"{str(test_dir)}/naruto"], grouped["e2e"])
        self.assertNotIn("kitchen", grouped)
        self.assertNotIn("kitchen-e2e", grouped)


class TestSetTag(unittest.TestCase):
    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1515"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "putsch"})
    def test_default(self):
        tags = junit.set_tags("agent-devx", "base", "", {}, "")
        self.assertEqual(len(tags), 18)
        self.assertIn("slack_channel:agent-devx-ops", tags)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1664"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "beer"})
    def test_flag(self):
        tags = junit.set_tags(
            "agent-devx",
            "base",
            'kitchen-e2e',
            ["upload_option.os_version_from_name"],
            "kitchen-rspec-win2016-azure-x86_64.xml",
        )
        self.assertEqual(len(tags), 22)
        self.assertIn("e2e_internal_error:true", tags)
        self.assertIn("version:win2016", tags)
        self.assertNotIn("upload_option.os_version_from_name", tags)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1789"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "revolution"})
    def test_additional_tags(self):
        tags = junit.set_tags("agent-devx", "base", "", ["--tags", "simple:basique"], "")
        self.assertEqual(len(tags), 20)
        self.assertIn("simple:basique", tags)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1789"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "revolution"})
    def test_additional_tags_from_method(self):
        tags = junit.set_tags(
            "agent-devx", "base", "", junit.read_additional_tags(Path("tasks/unit_tests/testdata")), ""
        )
        self.assertEqual(len(tags), 18)


class TestJUnitUploadFromTGZ(unittest.TestCase):
    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1664"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "beer"})
    @patch("tasks.libs.common.junit_upload_core.check_call")
    @patch("tasks.libs.common.junit_upload_core.which")
    def test_success(self, mock_which, mock_check_call):
        mock_which.side_effect = lambda cmd: f"/usr/local/bin/{cmd}"
        junit.junit_upload_from_tgz(
            "tasks/unit_tests/testdata/testjunit-tests_deb-x64-py3.tgz",
            "tasks/unit_tests/testdata/test_output_no_failure.json",
        )
        mock_check_call.assert_called()
        self.assertEqual(mock_check_call.call_count, 29)
        tmp_dir_vars = ("HOME", "TEMP", "TMP", "TMPDIR")
        seen_tmp_dirs = set()
        for _, kwargs in mock_check_call.call_args_list:
            env = kwargs["env"]
            for k in tmp_dir_vars:
                self.assertIn(k, env)
            last_tmp_dir = env[tmp_dir_vars[-1]]
            self.assertDictEqual({k: env[k] for k in tmp_dir_vars}, {k: last_tmp_dir for k in tmp_dir_vars})
            self.assertNotIn(last_tmp_dir, seen_tmp_dirs)
            self.assertFalse(Path(last_tmp_dir).exists())
            seen_tmp_dirs.add(last_tmp_dir)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1664"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "beer"})
    @patch("tasks.libs.common.junit_upload_core.check_call")
    @patch("tasks.libs.common.junit_upload_core.which")
    def test_failure(self, mock_which, mock_check_call):
        def raise_on_every_second_call(*args, **kwargs):
            if mock_check_call.call_count % 2 == 0:
                raise CalledProcessError(1, args[0])

        mock_check_call.side_effect = raise_on_every_second_call
        mock_which.side_effect = lambda cmd: f"/usr/local/bin/{cmd}"
        with self.assertRaises(ExceptionGroup) as eg:
            junit.junit_upload_from_tgz(
                "tasks/unit_tests/testdata/testjunit-tests_deb-x64-py3.tgz",
                "tasks/unit_tests/testdata/test_output_no_failure.json",
            )
        mock_check_call.assert_called()
        self.assertEqual(mock_check_call.call_count, 29)
        self.assertEqual(eg.exception.message, "14 junit uploads failed")
        self.assertEqual(len(eg.exception.exceptions), 14)
        for _, kwargs in mock_check_call.call_args_list:
            self.assertFalse(Path(kwargs["env"]["TMPDIR"]).exists())
