import shutil
import tempfile
import unittest

import yaml

from tasks.collector import YAMLValidationError, strip_invalid_components, validate_manifest

# Unit tests to check collector tasks for converged agent


class TestStripComponents(unittest.TestCase):
    def setUp(self):
        self.test_dir = tempfile.mkdtemp()
        self.test_file_path = f"{self.test_dir}/manifest.yaml"

    def tearDown(self):
        shutil.rmtree(self.test_dir)

    def test_remove_datadogconnector(self):
        shutil.copyfile("./tasks/unit_tests/testdata/collector/datadogconnector_manifest.yaml", self.test_file_path)
        strip_invalid_components(self.test_file_path, ["datadogconnector"])
        with open(self.test_file_path) as file:
            for line in file:
                if "datadogconnector" in line:
                    self.fail("datadogconnector was not successfully removed")

    def test_remove_datadogexporter(self):
        shutil.copyfile("./tasks/unit_tests/testdata/collector/datadogexporter_manifest.yaml", self.test_file_path)
        strip_invalid_components(self.test_file_path, ["datadogexporter"])
        with open(self.test_file_path) as file:
            for line in file:
                if "datadogexporter" in line:
                    self.fail("datadogexporter was not successfully removed")

    def test_remove_awscontainerinsightreceiver(self):
        shutil.copyfile(
            "./tasks/unit_tests/testdata/collector/awscontainerinsightreceiver_manifest.yaml", self.test_file_path
        )
        strip_invalid_components(self.test_file_path, ["awscontainerinsightreceiver"])
        with open(self.test_file_path) as file:
            for line in file:
                if "awscontainerinsightreceiver" in line:
                    self.fail("awscontainerinsightreceiver was not successfully removed")


class TestValidateYAML(unittest.TestCase):
    def setUp(self):
        self.test_dir = tempfile.mkdtemp()
        self.test_file_path = f"{self.test_dir}/manifest.yaml"

    def tearDown(self):
        shutil.rmtree(self.test_dir)

    def test_valid_manifest(self):
        with open("./tasks/unit_tests/testdata/collector/valid_datadog_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            self.assertEqual(validate_manifest(mock_manifest), [])

    def test_no_specified_version(self):
        with open("./tasks/unit_tests/testdata/collector/valid_manifest_without_specified_version.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            self.assertEqual(validate_manifest(mock_manifest), [])

    def test_outdated_version(self):
        with open("./tasks/unit_tests/testdata/collector/outdated_version_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            with self.assertRaises(YAMLValidationError):
                validate_manifest(mock_manifest)

    def test_mismatched_versions(self):
        with open("./tasks/unit_tests/testdata/collector/mismatched_versions_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            with self.assertRaises(YAMLValidationError):
                validate_manifest(mock_manifest)

    def test_healthcheckextension(self):
        with open("./tasks/unit_tests/testdata/collector/healthcheckextension_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            with self.assertRaises(YAMLValidationError):
                validate_manifest(mock_manifest)

    def test_pprofextension(self):
        with open("./tasks/unit_tests/testdata/collector/pprofextension_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            with self.assertRaises(YAMLValidationError):
                validate_manifest(mock_manifest)

    def test_prometheusreceiver(self):
        with open("./tasks/unit_tests/testdata/collector/prometheusreceiver_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            with self.assertRaises(YAMLValidationError):
                validate_manifest(mock_manifest)

    def test_zpagesextension(self):
        with open("./tasks/unit_tests/testdata/collector/zpagesextension_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            with self.assertRaises(YAMLValidationError):
                validate_manifest(mock_manifest)

    def test_datadogconnector(self):
        with open("./tasks/unit_tests/testdata/collector/datadogconnector_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            self.assertEqual(validate_manifest(mock_manifest), ["datadogconnector"])

    def test_datadogexporter(self):
        with open("./tasks/unit_tests/testdata/collector/datadogexporter_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            self.assertEqual(validate_manifest(mock_manifest), ["datadogexporter"])

    def test_awscontainerinsightreceiver(self):
        with open("./tasks/unit_tests/testdata/collector/awscontainerinsightreceiver_manifest.yaml") as mock_file:
            mock_manifest = yaml.safe_load(mock_file)
            self.assertEqual(validate_manifest(mock_manifest), ["awscontainerinsightreceiver"])
