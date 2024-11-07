import shutil
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

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
        self.assertEqual(junit.split_junitxml(xml_file.parent, xml_file, owners, []), 1)
        generated_folder = xml_file.parent / "windows-agent_base"
        self.assertTrue(generated_folder.exists())

    def test_with_split(self):
        xml_file = Path("./tasks/unit_tests/testdata/secret.tar.gz/-go-src-datadog-agent-junit-out-base.xml")
        owners = read_owners(".github/CODEOWNERS")
        self.assertEqual(junit.split_junitxml(xml_file.parent, xml_file, owners, []), 29)


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

    def test_e2e_kitchen(self):
        test_dir = Path("./tasks/unit_tests/testdata/to_group")
        grouped = junit.group_per_tags(test_dir, ["upload_option.os_version_from_name"])
        self.assertNotIn("default", grouped)
        self.assertIn("kitchen", grouped)
        self.assertCountEqual([f"{str(test_dir)}/onepiece", f"{str(test_dir)}/dragonball"], grouped["kitchen"])
        self.assertIn("kitchen-e2e", grouped)
        self.assertEqual([f"{str(test_dir)}/naruto"], grouped["kitchen-e2e"])
        self.assertNotIn("e2e", grouped)


class TestSetTag(unittest.TestCase):
    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1515"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "putsch"})
    def test_default(self):
        tags = junit.set_tags("agent-devx-infra", "base", "", {}, "")
        self.assertEqual(len(tags), 14)
        self.assertIn("slack_channel:agent-devx-ops", tags)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1664"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "beer"})
    def test_flag(self):
        tags = junit.set_tags(
            "agent-devx-infra",
            "base",
            'kitchen-e2e',
            ["upload_option.os_version_from_name"],
            "kitchen-rspec-win2016-azure-x86_64.xml",
        )
        self.assertEqual(len(tags), 18)
        self.assertIn("e2e_internal_error:true", tags)
        self.assertIn("version:win2016", tags)
        self.assertNotIn("upload_option.os_version_from_name", tags)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1789"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "revolution"})
    def test_additional_tags(self):
        tags = junit.set_tags("agent-devx-infra", "base", "", ["--tags", "simple:basique"], "")
        self.assertEqual(len(tags), 16)
        self.assertIn("simple:basique", tags)

    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1789"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "revolution"})
    def test_additional_tags_from_method(self):
        tags = junit.set_tags(
            "agent-devx-infra", "base", "", junit.read_additional_tags(Path("tasks/unit_tests/testdata")), ""
        )
        self.assertEqual(len(tags), 14)


class TestJUnitUploadFromTGZ(unittest.TestCase):
    @patch.dict("os.environ", {"CI_PIPELINE_ID": "1664"})
    @patch.dict("os.environ", {"CI_PIPELINE_SOURCE": "beer"})
    @patch("tasks.libs.common.junit_upload_core.Popen")
    def test_e2e(self, mock_popen):
        mock_instance = MagicMock()
        mock_instance.communicate.return_value = (b"stdout", b"")
        mock_popen.return_value = mock_instance
        junit.junit_upload_from_tgz("tasks/unit_tests/testdata/testjunit-tests_deb-x64-py3.tgz")
        mock_popen.assert_called()
        self.assertEqual(mock_popen.call_count, 31)
